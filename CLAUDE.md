# SLOTH-GO

Rule-driven file archiver: moves files from input folders into dated/organized archive folders and deletes old archives. Runs in production on a Windows server as a scheduled task. Single `main` package.

## Files
- `SlothGO.go` — config loading/migration, rule validation, move workers, delete pass, folder types
- `balance.go` — round-robin `Balancer` across multiple output folders
- `logger.go` — `AppLogger` (Info/Warn/Error + counters); daily rotating `logs/sloth-YYYYMMDD.log`, 30-day retention, `logs/sloth.log` links to current
- `rules_test.go` — regression tests for rule behavior; `integration_test.go`, `dryrun*_test.go`, `SlothGo_test.go`

## Commands
- `make test` / `go test ./...` — run with `-race` when touching workers
- `go vet ./...` and `GOOS=windows go vet ./...`
- `make build-win` — console exe for manual runs
- `make build-win-headless` — `-H=windowsgui` exe for Task Scheduler (no console window)
- `go run . --dry-run` / `SLOTH_DRY_RUN=1`, `go run . -list-types`

CI tests Go 1.21–1.23: don't use newer stdlib APIs (e.g. `t.Chdir`, `for range n`).

## Config (`config.json`, gitignored, read from the working directory)
Array of rules: `name`, `input`, `output` (array), `extension` (case-insensitive, `""` = all), `folderType`, `deleteOlderThan` (days, 0 = off), `minAgeMinutes` (default 5 when absent, 0 = off), `dryRun`.
- Folder types `1`–`7` (see `folderTypeDescriptions`) plus `delete` (delete-only, recursive over `input`). Type `1` folders are unpadded (`2026/9/Day 5`) — keep it that way; existing archives depend on it.
- Paths may use forward slashes (`U:/Archive`); `filepath.Clean` converts on Windows. Write-back uses forward slashes.
- Legacy `removeOlderThan` is migrated to `deleteOlderThan` and the file rewritten once; never rewrite otherwise.

## Behavior that must hold
- Dry-run changes nothing on disk: no moves, deletes, folder creation, empty-folder pruning, or config rewrite.
- Never overwrite in the archive: `reserveDestination` adds `_1`, `_2` suffixes (os.Rename replaces files on Windows).
- Missing input folder → Warn and skip the rule. Invalid rules → Error and skip before touching files.
- A failure on one file is logged and processing continues.
- Empty-folder pruning never removes a delete root or any path in `configuredPaths`.
- Log via `AppLogger`, not `log`; Errors print one line to stderr (no stack traces).

## Deployment gotchas
- Production paths are on mapped drive `U:`. Mapped drives don't exist for tasks set to "Run whether user is logged on or not" — use UNC paths (`//server/share/...`) if that ever changes.
- The task's "Start in" must be the exe folder: `config.json` and `logs/` are relative to the working directory, and the headless build has no console to show failures.
- "The system cannot find the file specified" on a folder usually means the last path component is missing; "path specified" means a parent or drive is missing.

## Tests
Tests `chdir` into `t.TempDir()` (see `inTempDir`) because config and logs are cwd-relative. Add a regression test in `rules_test.go` for any behavior change.
