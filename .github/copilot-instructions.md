# Copilot Instructions for SLOTH-GO

## Project Overview
- **SLOTH-GO** is a fast file mover written in Go, designed to move or delete files based on rules defined in `config.json`.
- The main entry point is `SlothGO.go`, which reads configuration, then processes files in parallel using goroutines and a round-robin balancer (`balance.go`).
- The project is structured for batch file operations, supporting various output folder structures and file deletion by age.

## Key Files
- `SlothGO.go`: Main logic, file moving, deletion, and worker orchestration.
- `balance.go`: Implements a thread-safe round-robin balancer for distributing output folders.
- `config.json`: User-supplied configuration for file operations (input/output paths, extension, folderType, etc.).
- `README.md`: Explains configuration and folderType options.

## Configuration
- All file operations are driven by `config.json`, which is an array of rules. Each rule includes:
  - `name`: Description
  - `input`: Source directory
  - `output`: Array of destination directories
  - `extension`: File extension to match (e.g., `.pdf`)
  - `folderType`: Output structure (see below)
  - `removeOlderThan`: (Optional) Days old for deletion (used with `folderType: delete`)
- FolderType options:
  - `1`: By modified date (YYYY/MM/Day DD)
  - `2`: By file extension
  - `3`: By extension then year
  - `4`: Simple move to output root
  - `5`: YYYYMM as folder
  - `delete`: Delete files older than `removeOlderThan` days

## Developer Workflows
- **Build:**
  - Use Makefile: `make build` or standard Go: `go build .`
  - Cross-platform builds: `make build` includes multiple architectures
- **Run:**
  - `make run` or `go run .` (ensure `config.json` is present)
- **Test:**
  - `make test` or `go test -v ./...` (includes comprehensive unit tests)
  - Coverage: `make test-coverage`
- **Debug:**
  - Use Go debugging tools; main logic is in `main()` in `SlothGO.go`
- **Lint:**
  - `make lint` (requires golangci-lint installation)
- **CI/CD:**
  - GitHub Actions configured for automated testing and building

## Patterns & Conventions
- Uses goroutines and a `sync.WaitGroup` for parallel file operations.
- File moving is distributed using a round-robin balancer (`Balancer.Next`).
- All configuration is externalized in `config.json`—do not hardcode paths or rules.
- Logging is via the standard `log` package.
- File deletions use `filepath.Walk` and check file age.

## Integration Points
- No external dependencies beyond the Go standard library
- Go modules enabled (`go.mod`) with minimum Go 1.21 requirement
- Designed for local filesystem operations only
- Uses modern Go APIs (os.ReadFile, filepath.WalkDir)
- GitHub Actions for CI/CD automation

## Examples
- See `README.md` and `config.json` for configuration samples and folderType explanations.
- Example config rule:
  ```json
  {
    "name": "Test Archive",
    "input": "/path/to/source",
    "output": ["/path/to/dest"],
    "extension": ".pdf",
    "folderType": "1",
    "removeOlderThan": 0
  }
  ```

## Recommendations for AI Agents
- Always read and respect `config.json` for operational rules.
- When adding new features, follow the pattern of external configuration.
- Keep concurrency patterns (goroutines, channels, WaitGroup) consistent with existing code.
- Update `README.md` and this file with any new conventions or workflow changes.
