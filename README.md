# SLOTH-GO

Rule-driven file archiver written in Go. Each rule moves matching files from an input folder into an organized archive (by date, extension, or flat) and can delete archived files after a set number of days. Built for unattended runs from Windows Task Scheduler.

[![CI](https://github.com/bird2920/SLOTH-GO/workflows/CI/badge.svg)](https://github.com/bird2920/SLOTH-GO/actions)

## Features

- **Archive layouts**: 7 folder types based on modified date, extension, or flat output
- **Retention**: delete archived files older than N days, then remove the empty folders left behind
- **Never overwrites**: name collisions in the archive are saved as `name_1.ext`, `name_2.ext`, ...
- **Safe with files in progress**: files modified in the last few minutes wait for the next run
- **Dry-run**: simulate a run globally or per rule with no filesystem changes
- **Parallel moves**: worker goroutines with round-robin balancing across multiple output folders
- **Daily logs**: Info/Warn/Error log files kept for 30 days
- **Headless Windows build**: no console window when run from a scheduled task

## Quick Start

```bash
make build                 # build for the current OS
make build-win             # Windows console exe: bin/sloth-go.exe
make build-win-headless    # Windows exe with no console window: bin/sloth-go-headless.exe

go run . --dry-run         # simulate using ./config.json
go run .                   # run for real
go run . -list-types       # print folder types
```

### Command-Line Options

| Option | Description |
|--------|-------------|
| `--dry-run` | Simulate every rule; nothing on disk changes |
| `-list-types` | Print the available folder types and exit |
| `-version` | Print the build version and exit (also logged as `Version:` at the start of every run) |
| `SLOTH_DRY_RUN=1` (env var) | Same as `--dry-run` |

`config.json` is read from, and `logs/` is written to, the **current working directory**.

## Configuration

`config.json` is an array of rules, processed one at a time in the order listed:

```json
[
  {
    "name": "Invoice Archive",
    "input": "U:/Processed/Invoices",
    "output": ["U:/Processed/Invoices/Archive"],
    "extension": ".TIF",
    "folderType": "1",
    "deleteOlderThan": 60,
    "dryRun": false
  },
  {
    "name": "Forms Log Cleanup",
    "input": "C:/Program Files (x86)/Vendor/Logs/Archive",
    "output": ["C:/Program Files (x86)/Vendor/Logs/Archive"],
    "extension": ".csv",
    "folderType": "delete",
    "deleteOlderThan": 30
  }
]
```

### Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | Yes | | Rule name shown in the log |
| `input` | Yes | | Folder to scan. Only files directly in this folder are moved (not subfolders) |
| `output` | Yes* | | Array of destination folders. With more than one, files are distributed round-robin. *Not used by `delete` rules |
| `extension` | Yes | | Extension to match, case-insensitive (`.TIF` matches `scan.tif`). `""` matches all files |
| `folderType` | Yes | | Archive layout (`1`–`7`), or `delete` for a delete-only rule. See [Folder Types](#folder-types) |
| `deleteOlderThan` | No | `0` | Delete matching files older than N days, then remove empty folders. Move rules clean their `output` folders; `delete` rules clean their `input`. Searches subfolders. `0` = off |
| `minAgeMinutes` | No | `5` | Leave files modified in the last N minutes for the next run, so files still being written aren't moved. `0` = off. Ignored by `delete` rules |
| `dryRun` | No | `false` | Simulate this rule only |

File age for `deleteOlderThan` and `minAgeMinutes` comes from the file's **last-modified time**. Moving a file keeps that time, so a file that sat in the input folder for 20 days before being archived counts as 20 days old.

### Windows Paths

Use forward slashes so paths need no escaping. They are converted to backslashes when the config loads:

```json
"input": "U:/Processed File Archive/DriverPay",
"output": ["//fileserver/share/DriverPay/Archive"]
```

Escaped backslashes (`"U:\\Processed File Archive"`) also work.

### Folder Types

| Type | Layout | Example |
|------|--------|---------|
| `1` | Year / month / day from modified date (month and day not zero-padded) | `2026/9/Day 5/` |
| `2` | By extension | `TIF/` |
| `3` | Extension, then year | `TIF/2026/` |
| `4` | Flat: output folder root | `Archive/` |
| `5` | Year-month | `202609/` |
| `6` | Year / month / day, zero-padded (sorts correctly) | `2026/09/05/` |
| `7` | ISO year / week (Dec 30, 2025 → `2026/Week 01`) | `2026/Week 36/` |
| `delete` | Delete only: no moves; deletes old files from `input` and its subfolders | |

Types `2` and `3` are most useful with `"extension": ""`. With a single extension they always produce one subfolder. For new date-based rules, prefer `6` over `1` so folders sort in order.

### Rule Behavior

- **Missing input folder**: logged as a warning; the rule is skipped and the run continues.
- **Invalid rules**: an unknown `folderType`, an empty `output`, a `delete` rule without `deleteOlderThan`, or type `4` writing back into its own input are logged as errors and skipped before any file is touched.
- **Output folders**: created automatically when their parent folder exists. If the parent is missing too, the rule logs an error and stops.
- **Name collisions**: if `report.txt` already exists in the destination, the new file is saved as `report_1.txt` (then `_2`, ...). Archived files are never overwritten.
- **Per-file failures**: a file that can't be moved or deleted is logged, and the rule continues with the rest.
- **Empty-folder cleanup**: after deleting old files, empty folders under that path are removed. The path itself and any folder another rule uses as input or output are always kept.
- **Moves must stay on one drive**: moves are renames, so `input` and `output` must be on the same drive or share. A move from `C:` to `U:` fails for each file with an error.

### Multiple Output Folders

```json
"output": ["//nas1/archive", "//nas2/archive", "//nas3/archive"]
```

Files are distributed round-robin: first → `nas1`, second → `nas2`, third → `nas3`, fourth → `nas1`, and so on.

## Dry-Run Mode

Enable with `--dry-run`, `SLOTH_DRY_RUN=1`, or `"dryRun": true` on individual rules. Dry-run makes **no** changes: no moves, deletes, folder creation, empty-folder removal, or `config.json` rewrite. It logs a sample of up to 5 moves and 5 deletes per rule, plus counts:

```
[DRY-RUN] [Rule:Invoices] Would create output folder: U:\Processed\Invoices\Archive
[DRY-RUN] Would move U:\Processed\Invoices\a.TIF -> U:\Processed\Invoices\Archive\2026\9\Day 30\a.TIF
[DRY-RUN] Would delete: U:\Processed\Invoices\Archive\2026\7\Day 1\old.TIF
[DRY-RUN] ...and 212 more files would be deleted from U:\Processed\Invoices\Archive
[DRY-RUN] Would remove 14 empty folders under U:\Processed\Invoices\Archive
```

Rules that depend on each other only fully show up in dry-run once the first one has actually run. For example, a rule that reads another rule's output folder warns "Input folder not found" until that folder exists.

## Logging

Logs are written to `logs/sloth-YYYYMMDD.log`, one file per day, and files older than 30 days are removed automatically. `logs/sloth.log` points to the current day's file.

| Level | Used for | Where |
|-------|----------|-------|
| INFO | Rule start/finish, moves held back by `minAgeMinutes`, deletions, renamed collisions, dry-run actions | Log file |
| WARN | Missing input folders, unreadable files or folders | Log file |
| ERROR | Invalid rules, failed moves/deletes | Log file and stderr (one line each) |

Each run ends with a summary in the log and a completion line on the console:

```
SUMMARY: rules=7 files=127 warnings=0 errors=0 elapsed=2.450s dryRun=false
Sloth: Completed in 2.45s (dryRun=false, warnings=0, errors=0). See detailed log: logs/sloth.log
```

`files` counts files moved (or that would be moved in dry-run).

## Running from Windows Task Scheduler

1. Build with `make build-win-headless` and copy `bin/sloth-go-headless.exe` and `config.json` into one folder on the server.
2. Create a task whose action runs `sloth-go-headless.exe`, with **Start in** set to that folder. Without it, the task runs from `C:\Windows\System32` and can't find `config.json`.
3. If `config.json` uses **mapped drives** (e.g. `U:`), choose **Run only when user is logged on**. With "Run whether user is logged on or not", mapped drives don't exist and every rule reports its input as missing. To run without a logged-on user, switch the config to UNC paths (`//server/share/...`).

The headless build has no console, so all output goes to `logs/`. Use `sloth-go.exe` for manual runs where you want to see the console output.

`make` stamps the version from `git describe` (tag releases, e.g. `git tag v2.1.0`, for readable versions). The headless exe can't print `-version` to a console, so check the `Version:` line in the log instead.

### Inventory of installs across servers

`scripts/Collect-SlothConfigs.ps1` reads every SLOTH install on a list of servers without changing anything. It finds scheduled tasks that run a sloth exe, then copies each `config.json` and records the task settings, exe hash, logged version and config problems (invalid JSON, legacy `removeOlderThan`, drive-relative paths such as `F:BMI`):

```powershell
.\scripts\Collect-SlothConfigs.ps1 -ComputerName SERVER01,SERVER02 -OutputPath C:\repos\sloth-go-org\deployments
```

Run it as an admin on the servers (it uses `\\SERVER\C$` shares and remote Task Scheduler). Installs with no task go in a CSV (`Server,InstallPath`) passed as `-InventoryCsv`. Server configs contain internal paths, so `deployments/` is gitignored here; keep them in the internal repo.

## Legacy Config Migration

Older configs used `removeOlderThan`, and marked delete rules by putting `DELETE` in the name. On startup these are converted, and `config.json` is rewritten **once** (with forward-slash paths):

- `removeOlderThan` becomes `deleteOlderThan` (an existing `deleteOlderThan` wins) and the old field is removed.
- A rule with no `folderType` whose name contains `DELETE` becomes `"folderType": "delete"`.

Delete rules remain separate rules. The rewrite never happens in dry-run, and configs already in the current format are never rewritten.

## Troubleshooting

- **Start with the log**: `logs/sloth.log`. The console line shows warning and error counts.
- **"Input folder not found, skipping"**: the path in `input` doesn't exist as seen by the process. Check spelling and trailing spaces, and whether a mapped drive exists in the task's context.
- **Windows "cannot find the file specified" vs "cannot find the path specified"**: *file* means the parent folder exists but the last folder in the path doesn't. *Path* means a parent folder or the drive itself is missing.
- **Files not moving**: check the extension, whether the files are in a subfolder (moves don't search subfolders), and whether they were modified within `minAgeMinutes`.
- **Try changes safely**: run with `--dry-run` first and review the log.

## Development

```bash
make test            # go test -v ./...
go test -race ./...  # recommended when touching the move workers
make lint            # golangci-lint (config in .golangci.yml)
make fmt
make test-coverage
```

Requires Go 1.21+. CI tests Go 1.21, 1.22, and 1.23.

`master` is protected: changes go through a pull request.

1. Create a branch
2. Run `make test` and `make lint`
3. Open a pull request

## License

See [LICENSE](LICENSE).

## Additional Resources

- [CLAUDE.md](CLAUDE.md): guidance for AI coding agents
- [MODERNIZATION.md](MODERNIZATION.md): history of the Go modernization work
