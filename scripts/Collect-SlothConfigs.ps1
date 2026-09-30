<#
.SYNOPSIS
    Collects config.json and install details from every SLOTH-GO install on a list of servers.

.DESCRIPTION
    Read-only: nothing on the servers is changed. For each server the script finds the
    scheduled tasks that run a sloth exe, then reads each install over the admin share
    (\\SERVER\C$\...). It writes:

        <OutputPath>\<SERVER>\<instance>\config.json   copy of the live config
        <OutputPath>\<SERVER>\<instance>\install.json  task, exe hash, version, config checks
        <OutputPath>\inventory.csv                     one row per install

    Run it from an account that is an admin on the servers (admin shares + remote Task
    Scheduler access). Works in Windows PowerShell 5.1.

    Installs that have no scheduled task, or servers where remote task lookup is blocked,
    can be listed in a CSV with columns Server,InstallPath (InstallPath is the folder that
    holds the exe and config.json). Rows with a blank InstallPath are discovered from tasks.

.EXAMPLE
    .\Collect-SlothConfigs.ps1 -ComputerName SERVER01,SERVER02 -OutputPath C:\repos\sloth-go-org\deployments

.EXAMPLE
    .\Collect-SlothConfigs.ps1 -InventoryCsv .\servers.csv -OutputPath C:\repos\sloth-go-org\deployments
#>
[CmdletBinding()]
param(
    [string[]]$ComputerName,
    [string]$InventoryCsv,
    [Parameter(Mandatory)]
    [string]$OutputPath,
    # Task actions whose exe path matches this pattern are treated as SLOTH installs.
    [string]$ExePattern = 'sloth'
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

# ---- helpers ---------------------------------------------------------------------------

function ConvertTo-AdminSharePath([string]$Server, [string]$LocalPath) {
    # C:\Tools\Sloth -> \\SERVER\C$\Tools\Sloth ; UNC paths pass through unchanged.
    $p = $LocalPath.Trim().Trim('"')
    $p = $p -replace '%ProgramFiles\(x86\)%', 'C:\Program Files (x86)'
    $p = $p -replace '%ProgramFiles%', 'C:\Program Files'
    $p = $p -replace '%SystemDrive%', 'C:'
    if ($p -match '^\\\\') { return $p }
    if ($p -match '^([A-Za-z]):\\?(.*)$') {
        return "\\$Server\$($Matches[1].ToUpper())`$\$($Matches[2])"
    }
    return $null
}

function Get-SafeName([string]$Name) {
    ($Name.Trim() -replace '[\\/:*?"<>|\s]+', '_').Trim('_')
}

function Get-TriggerSummary($Task) {
    $parts = foreach ($t in @($Task.Triggers)) {
        if ($null -eq $t) { continue }
        $kind = $t.CimClass.CimClassName -replace '^MSFT_Task', '' -replace 'Trigger$', ''
        $at = ''
        if ($t.StartBoundary) { $at = ' at ' + ([datetime]$t.StartBoundary).ToString('HH:mm') }
        "$kind$at"
    }
    ($parts -join '; ')
}

function Test-SlothConfig([string]$Path) {
    # Light sanity checks so broken or legacy configs stand out in the inventory.
    $result = [ordered]@{ Valid = $false; RuleCount = 0; Issues = @() }
    try {
        # Assign first: PowerShell 5.1 emits a JSON array as one object, so @(...) directly would count 1.
        $parsed = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
        $rules = @($parsed)
    } catch {
        $result.Issues += "Invalid JSON: $($_.Exception.Message)"
        return $result
    }
    $result.Valid = $true
    $result.RuleCount = $rules.Count
    foreach ($r in $rules) {
        $name = if ($r.PSObject.Properties['name']) { $r.name } else { '(unnamed)' }
        if ($r.PSObject.Properties['removeOlderThan']) {
            $result.Issues += "${name}: legacy removeOlderThan (new builds rewrite config once)"
        }
        if ($r.PSObject.Properties['output'] -and $r.output -is [string]) {
            $result.Issues += "${name}: output is a string, not an array"
        }
        $paths = @()
        if ($r.PSObject.Properties['input']) { $paths += $r.input }
        if ($r.PSObject.Properties['output']) { $paths += @($r.output) }
        foreach ($p in $paths) {
            if ($p -is [string] -and $p -match '^[A-Za-z]:[^\\/]') {
                $result.Issues += "${name}: drive-relative path '$p' (missing slash after drive)"
            }
        }
    }
    return $result
}

function Get-LoggedVersion([string]$InstallUnc) {
    # Builds from the version-stamp change onward log "Version: <x>" at startup.
    $logDir = Join-Path $InstallUnc 'logs'
    if (-not (Test-Path -LiteralPath $logDir)) { return 'unknown (no logs folder)' }
    $latest = Get-ChildItem -LiteralPath $logDir -Filter 'sloth*.log' -File -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if (-not $latest) { return 'unknown (no log files)' }
    $hit = Select-String -LiteralPath $latest.FullName -Pattern 'INFO: Version: (\S+)' |
        Select-Object -Last 1
    if ($hit) { return $hit.Matches[0].Groups[1].Value }
    return 'unknown (older build, no version line)'
}

# ---- build the list of installs to read -------------------------------------------------

$targets = New-Object System.Collections.Generic.List[object]
if ($ComputerName) {
    foreach ($c in $ComputerName) { $targets.Add([pscustomobject]@{ Server = $c; InstallPath = '' }) }
}
if ($InventoryCsv) {
    foreach ($row in Import-Csv -LiteralPath $InventoryCsv) {
        $ip = ''
        if ($row.PSObject.Properties['InstallPath']) { $ip = "$($row.InstallPath)" }
        $targets.Add([pscustomobject]@{ Server = $row.Server.Trim(); InstallPath = $ip.Trim() })
    }
}
if ($targets.Count -eq 0) { throw 'Pass -ComputerName and/or -InventoryCsv.' }

$installs = New-Object System.Collections.Generic.List[object]
$discovered = @{}

foreach ($server in ($targets | Select-Object -ExpandProperty Server -Unique)) {
    if ($discovered.ContainsKey($server)) { continue }
    $discovered[$server] = $true
    Write-Host "[$server] looking for SLOTH scheduled tasks..."
    try {
        $cim = New-CimSession -ComputerName $server
        try {
            $tasks = Get-ScheduledTask -CimSession $cim | Where-Object {
                @($_.Actions | Where-Object { $_.PSObject.Properties['Execute'] -and $_.Execute -match $ExePattern }).Count -gt 0
            }
            foreach ($task in @($tasks)) {
                $info = Get-ScheduledTaskInfo -CimSession $cim -TaskName $task.TaskName -TaskPath $task.TaskPath
                foreach ($a in @($task.Actions | Where-Object { $_.PSObject.Properties['Execute'] -and $_.Execute -match $ExePattern })) {
                    $installs.Add([pscustomobject]@{
                        Server           = $server
                        Instance         = $task.TaskName
                        Source           = 'scheduled task'
                        TaskName         = $task.TaskPath + $task.TaskName
                        TaskState        = "$($task.State)"
                        RunAs            = $task.Principal.UserId
                        LogonType        = "$($task.Principal.LogonType)"
                        RunLevel         = "$($task.Principal.RunLevel)"
                        Schedule         = Get-TriggerSummary $task
                        LastRunTime      = $info.LastRunTime
                        LastTaskResult   = $info.LastTaskResult
                        Exe              = $a.Execute.Trim('"')
                        Arguments        = $a.Arguments
                        StartIn          = $a.WorkingDirectory
                    })
                }
            }
            if (@($tasks).Count -eq 0) { Write-Warning "[$server] no scheduled task runs an exe matching '$ExePattern'" }
        } finally {
            Remove-CimSession $cim
        }
    } catch {
        Write-Warning "[$server] task lookup failed: $($_.Exception.Message). List its install in -InventoryCsv."
    }
}

# Explicit install paths from the CSV (skipped if a task already points at the same folder).
foreach ($t in $targets | Where-Object { $_.InstallPath }) {
    $already = $installs | Where-Object {
        $_.Server -eq $t.Server -and (($_.StartIn -eq $t.InstallPath) -or ((Split-Path $_.Exe) -eq $t.InstallPath))
    }
    if ($already) { continue }
    $installs.Add([pscustomobject]@{
        Server = $t.Server; Instance = (Split-Path $t.InstallPath -Leaf); Source = 'inventory csv'
        TaskName = ''; TaskState = ''; RunAs = ''; LogonType = ''; RunLevel = ''; Schedule = ''
        LastRunTime = $null; LastTaskResult = $null; Exe = ''; Arguments = ''; StartIn = $t.InstallPath
    })
}

# ---- read each install ------------------------------------------------------------------

New-Item -ItemType Directory -Force -Path $OutputPath | Out-Null
$rows = New-Object System.Collections.Generic.List[object]
$collectedAt = (Get-Date).ToString('s')

foreach ($i in $installs) {
    $warnings = @()
    $label = "[$($i.Server)] $($i.Instance)"

    # config.json and logs\ are read from "Start in"; that is where the exe reads them too.
    $installLocal = $i.StartIn
    if (-not $installLocal) {
        $warnings += 'Start in is blank: the exe runs from C:\Windows\System32 and will not find config.json'
        if ($i.Exe) { $installLocal = Split-Path $i.Exe }
    }
    if ($installLocal -match '^[A-Za-z]:' -and $installLocal -notmatch '^[A-Za-z]:\\') {
        $warnings += "Start in '$installLocal' is drive-relative"
    }
    $installUnc = if ($installLocal) { ConvertTo-AdminSharePath $i.Server $installLocal } else { $null }
    $exeUnc = if ($i.Exe) { ConvertTo-AdminSharePath $i.Server $i.Exe } else { $null }

    if ($i.LogonType -eq 'Interactive' -or $i.LogonType -eq 'InteractiveOrPassword') {
        $warnings += 'Task runs only when its user is logged on'
    }
    if ($i.RunLevel -eq 'Highest') {
        $warnings += 'Task runs elevated: mapped drives from the user session are not visible'
    }

    $dest = Join-Path (Join-Path $OutputPath (Get-SafeName $i.Server)) (Get-SafeName $i.Instance)
    New-Item -ItemType Directory -Force -Path $dest | Out-Null

    $exeHash = ''; $exeModified = ''
    if ($exeUnc -and (Test-Path -LiteralPath $exeUnc)) {
        $exeHash = (Get-FileHash -LiteralPath $exeUnc -Algorithm SHA256).Hash
        $exeModified = (Get-Item -LiteralPath $exeUnc).LastWriteTime.ToString('s')
    } elseif ($i.Exe) {
        $warnings += "Exe not reachable at $exeUnc"
    }

    $configUnc = if ($installUnc) { Join-Path $installUnc 'config.json' } else { $null }
    $check = [ordered]@{ Valid = $false; RuleCount = 0; Issues = @() }
    $configHash = ''; $configModified = ''
    if ($configUnc -and (Test-Path -LiteralPath $configUnc)) {
        Copy-Item -LiteralPath $configUnc -Destination (Join-Path $dest 'config.json') -Force
        $configHash = (Get-FileHash -LiteralPath $configUnc -Algorithm SHA256).Hash
        $configModified = (Get-Item -LiteralPath $configUnc).LastWriteTime.ToString('s')
        $check = Test-SlothConfig $configUnc
    } else {
        $warnings += "config.json not found at $configUnc"
    }

    $version = if ($installUnc) { Get-LoggedVersion $installUnc } else { 'unknown' }

    $record = [ordered]@{
        Server         = $i.Server
        Instance       = $i.Instance
        Source         = $i.Source
        CollectedAt    = $collectedAt
        TaskName       = $i.TaskName
        TaskState      = $i.TaskState
        RunAs          = $i.RunAs
        LogonType      = $i.LogonType
        RunLevel       = $i.RunLevel
        Schedule       = $i.Schedule
        LastRunTime    = if ($i.LastRunTime) { ([datetime]$i.LastRunTime).ToString('s') } else { '' }
        LastTaskResult = $i.LastTaskResult
        Exe            = $i.Exe
        Arguments      = $i.Arguments
        StartIn        = $i.StartIn
        Version        = $version
        ExeSha256      = $exeHash
        ExeModified    = $exeModified
        ConfigSha256   = $configHash
        ConfigModified = $configModified
        ConfigValid    = $check.Valid
        RuleCount      = $check.RuleCount
        ConfigIssues   = @($check.Issues)
        Warnings       = $warnings
    }
    $record | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $dest 'install.json') -Encoding UTF8

    $row = [pscustomobject]$record
    $row.ConfigIssues = ($check.Issues -join ' | ')
    $row.Warnings = ($warnings -join ' | ')
    $rows.Add($row)

    $status = if ($check.Valid) { "$($check.RuleCount) rules" } else { 'CONFIG PROBLEM' }
    Write-Host "$label -> $status, version $version"
    foreach ($w in @($warnings) + @($check.Issues)) { Write-Warning "$label $w" }
}

$csv = Join-Path $OutputPath 'inventory.csv'
$rows | Sort-Object Server, Instance | Export-Csv -LiteralPath $csv -NoTypeInformation -Encoding UTF8
Write-Host ""
Write-Host "Collected $($rows.Count) install(s) into $OutputPath"
Write-Host "Summary: $csv"
