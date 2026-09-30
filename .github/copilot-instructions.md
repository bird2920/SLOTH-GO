# Copilot Instructions for SLOTH-GO

## Project Overview
- **SLOTH-GO** is a fast file mover written in Go, designed to move or delete files based on rules defined in `config.json`.
- The main entry point is `SlothGO.go`, which reads configuration, then processes files in parallel using goroutines and a round-robin balancer (`balance.go`).
- The project is structured for batch file operations, supporting various output folder structures and file deletion by age.

## Key Files
- `SlothGO.go`: Main logic, file moving, deletion, and worker orchestration.
- `balance.go`: Implements a thread-safe round-robin balancer for distributing output folders.
- `logger.go`: Structured rotating logger with Info/Warn/Error levels and automatic cleanup.
- `config.json`: User-supplied configuration for file operations (input/output paths, extension, folderType, etc.).
- `README.md`: Explains configuration and folderType options.
- `MODERNIZATION.md`: Documents completed modernizations and Go best practices applied.

## Configuration
- All file operations are driven by `config.json`, which is an array of rules. Each rule includes:
  - `name`: Description
  - `input`: Source directory
  - `output`: Array of destination directories (load balanced via round-robin)
  - `extension`: File extension to match (e.g., `.pdf`). Use `""` for all files
  - `folderType`: Output structure (see below)
  - `deleteOlderThan`: (Optional) Delete files older than N days (0 = disabled)
  - `minAgeMinutes`: (Optional) Skip files modified in the last N minutes (default 5, 0 = disabled)
  - `dryRun`: (Optional) Enable dry-run mode for this rule only (default: false)
  - `removeOlderThan`: (Deprecated, legacy field) Use `deleteOlderThan` instead
- FolderType options:
  - `1`: By modified date (YYYY/MM/Day DD)
  - `2`: By file extension
  - `3`: By extension then year
  - `4`: Simple move to output root (flat)
  - `5`: YYYYMM as folder (compact date)
  - `6`: Standard Date (YYYY/MM/DD)
  - `7`: ISO Week (YYYY/Week NN)
  - `delete`: Delete-only mode (no moving, requires `deleteOlderThan` > 0)

## Developer Workflows
- **Build:**
  - Use Makefile: `make build` or standard Go: `go build .`
  - Cross-platform builds: `make build-win` for Windows
  - Scheduled tasks: `make build-win-headless` (no console window; output only in `logs/sloth.log`)
  - Output directory: `./bin/`
- **Run:**
  - `make run` or `go run .` (ensure `config.json` is present)
  - Dry-run mode: `go run . --dry-run` or `SLOTH_DRY_RUN=1 go run .`
  - List folder types: `go run . --list-types`
- **Test:**
  - `make test` or `go test -v ./...` (includes comprehensive unit tests)
  - Coverage: `make test-coverage` (generates HTML coverage report)
- **Debug:**
  - Use Go debugging tools; main logic is in `main()` in `SlothGO.go`
  - Check `logs/sloth.log` for detailed execution logs with rotation
- **Lint:**
  - `make lint` (requires golangci-lint installation)
  - Format code: `make fmt`
  - Vet code: `make vet`
- **CI/CD:**
  - GitHub Actions configured for automated testing and building
  - Test status badge in README

## Patterns & Conventions
- Uses goroutines and a `sync.WaitGroup` for parallel file operations.
- File moving is distributed using a round-robin balancer (`Balancer.Next`).
- All configuration is externalized in `config.json`—do not hardcode paths or rules.
- Config paths use forward slashes (`U:/Archive`); `filepath.Clean` converts them on Windows.
- Moves never overwrite: name collisions get `_1`, `_2` suffixes via `reserveDestination`.
- Logging uses a custom `AppLogger` with rotating log files (max 10MB, 5 backups, 30 day retention).
  - Info: High-level summaries and all dry-run operations
  - Warn: Missing input folders, unreadable files (file only, not stderr)
  - Error: Invalid rules and failed operations, one line each (file + stderr)
- File operations use modern Go APIs:
  - `filepath.WalkDir` for directory traversal (more efficient than deprecated `Walk`)
  - `os.ReadFile` instead of deprecated `ioutil.ReadFile`
  - `os.ReadDir` instead of deprecated `ioutil.ReadDir`
- Dry-run mode available globally (CLI flag or env var) and per-rule (config)
- Summary statistics tracked with atomic counters for thread-safety

## Integration Points
- External dependency: `github.com/lestrrat-go/file-rotatelogs` for rotating log functionality
- Go modules enabled (`go.mod`) with minimum Go 1.21 requirement
- Designed for local filesystem operations only
- Uses modern Go APIs (os.ReadFile, filepath.WalkDir, os.ReadDir)
- GitHub Actions for CI/CD automation
- Logs written to `logs/` directory with automatic rotation and gzip compression

## Examples
- See `README.md` and `config.json` for configuration samples and folderType explanations.
- Example config rule (with file deletion):
  ```json
  {
    "name": "Archive PDFs by Date",
    "input": "/path/to/source",
    "output": ["/path/to/dest1", "/path/to/dest2"],
    "extension": ".pdf",
    "folderType": "1",
    "deleteOlderThan": 90,
    "dryRun": false
  }
  ```
- Example delete-only rule:
  ```json
  {
    "name": "Cleanup Old Logs",
    "input": "/path/to/logs",
    "output": [],
    "extension": ".log",
    "folderType": "delete",
    "deleteOlderThan": 30
  }
  ```

## Recommendations for AI Agents
- Always read and respect `config.json` for operational rules.
- When adding new features, follow the pattern of external configuration.
- Keep concurrency patterns (goroutines, channels, WaitGroup) consistent with existing code.
- Use the custom `AppLogger` for all logging—do not use standard `log` directly.
- Maintain thread-safety with atomic operations for counters and synchronized access.
- Test new folder types extensively; add to `folderTypeDescriptions` map.
- Support dry-run mode for any new operations that modify the filesystem.
- Update `README.md`, `MODERNIZATION.md`, and this file with any new conventions or workflow changes.
- Follow Go 1.21+ best practices; avoid deprecated APIs.
- All tests should be comprehensive with coverage reporting enabled.
