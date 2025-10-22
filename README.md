# SLOTH-GO

Fast file mover written in Go with parallel processing, rotating logs, and dry-run simulation.

[![CI](https://github.com/bird2920/SLOTH-GO/workflows/CI/badge.svg)](https://github.com/bird2920/SLOTH-GO/actions)

## Features

- **Parallel Processing**: Uses goroutines with round-robin load balancing across multiple output directories
- **Dry-Run Mode**: Test configurations without making filesystem changes
- **Structured Logging**: Rotating logs with Info/Warn/Error levels and automatic cleanup
- **Flexible Organization**: 5 folder structure options based on date, extension, or custom patterns
- **Auto-Deletion**: Optional cleanup of old files based on age
- **Config Migration**: Automatically migrates legacy delete rules to new format

## Quick Start

### Build & Run

```bash
# Build
make build

# Run with default config
make run

# Run in dry-run mode (simulation only)
go run . --dry-run

# Or use environment variable
SLOTH_DRY_RUN=1 go run .
```

### Development

```bash
# Run tests
make test

# Test with coverage
make test-coverage

# Lint code
make lint

# Format code
make fmt
```

## Configuration

Create a `config.json` file with an array of rules:

```json
[
  {
    "name": "Archive PDFs by Date",
    "input": "/path/to/source",
    "output": ["/path/to/archive1", "/path/to/archive2"],
    "extension": ".pdf",
    "folderType": "1",
    "deleteOlderThan": 90,
    "dryRun": false
  },
  {
    "name": "Move Images",
    "input": "/path/to/photos",
    "output": ["/path/to/organized"],
    "extension": ".jpg",
    "folderType": "2"
  }
]
```

### Configuration Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Descriptive name for the rule |
| `input` | Yes | Source directory to scan for files |
| `output` | Yes | Array of destination directories (load balanced) |
| `extension` | Yes | File extension to match (e.g., `.pdf`, `.jpg`). Use `""` for all files |
| `folderType` | Yes | Output folder structure (see below) |
| `deleteOlderThan` | No | Delete files older than N days (0 = disabled) |
| `dryRun` | No | Enable dry-run for this rule only (default: false) |

### Folder Types

| Type | Pattern | Example Output |
|------|---------|----------------|
| `1` | By modified date | `2023/10/Day 15/` |
| `2` | By file extension | `pdf/` |
| `3` | Extension + year | `pdf/2023/` |
| `4` | Simple move (no subfolders) | `output/` |
| `5` | Year-month | `202310/` |

### Multiple Output Directories

When you specify multiple output directories, SLOTH-GO uses round-robin load balancing to distribute files evenly:

```json
{
  "output": ["/archive/drive1", "/archive/drive2", "/archive/drive3"]
}
```

Files will be alternated: first file → drive1, second → drive2, third → drive3, fourth → drive1, etc.

## Logging

Logs are written to `logs/sloth.log` with automatic rotation:

- **Max Size**: 10 MB per file
- **Max Backups**: 5 files retained
- **Max Age**: 30 days
- **Compression**: Old logs are gzipped

### Log Levels

- **Info**: High-level summaries (rule execution, file counts). In dry-run mode, logs every simulated action.
- **Warn**: Validation issues, unmatched migrations (file only)
- **Error**: Failures with stack traces (logged to file + printed to stderr)

### Summary Output

Each run ends with a summary line:
```
SUMMARY: rules=3 files=127 warnings=0 errors=0 elapsed=2.450s dryRun=false
```

## Dry-Run Mode

Test your configuration without making any changes:

```bash
# Command-line flag
go run . --dry-run

# Environment variable
SLOTH_DRY_RUN=1 go run .

# Per-rule in config.json
{
  "name": "Test Rule",
  "dryRun": true,
  ...
}
```

Dry-run mode logs all intended operations:
```
[DRY-RUN] Would create folder: /archive/2023/10/Day 15
[DRY-RUN] Would move /source/file.pdf -> /archive/2023/10/Day 15/file.pdf
[DRY-RUN] Would delete: /old/file.pdf
```

## Legacy Config Migration

SLOTH-GO automatically migrates old delete-style configs. Legacy format:

```json
[
  {
    "name": "Archive Files",
    "input": "/path",
    "output": ["/archive"],
    "extension": ".pdf",
    "folderType": "1"
  },
  {
    "name": "DELETE old files",
    "input": "/path",
    "output": ["/archive"],
    "extension": ".pdf",
    "folderType": "delete",
    "removeOlderThan": 60
  }
]
```

Automatically becomes:
```json
[
  {
    "name": "Archive Files",
    "input": "/path",
    "output": ["/archive"],
    "extension": ".pdf",
    "folderType": "1",
    "deleteOlderThan": 60
  }
]
```

Migration matches rules by `input + extension`. Unmatched delete rules generate warnings.

## Requirements

- Go 1.21 or higher
- Write permissions for source and destination directories
- Write permissions for `logs/` directory

## Examples

### Example 1: Archive by Date with Cleanup

```json
{
  "name": "Invoice Archive",
  "input": "/invoices/processed",
  "output": ["/archive/invoices"],
  "extension": ".pdf",
  "folderType": "1",
  "deleteOlderThan": 365
}
```

Organizes PDFs into `YYYY/MM/Day DD/` folders and deletes files older than 1 year.

### Example 2: Multi-Drive Photo Organization

```json
{
  "name": "Photo Backup",
  "input": "/photos/inbox",
  "output": ["/backup/disk1", "/backup/disk2", "/backup/disk3"],
  "extension": ".jpg",
  "folderType": "5"
}
```

Distributes photos across 3 drives in `YYYYMM/` folders.

### Example 3: Simple File Type Organization

```json
{
  "name": "Documents by Type",
  "input": "/downloads",
  "output": ["/organized"],
  "extension": ".docx",
  "folderType": "2"
}
```

Moves all `.docx` files into `organized/docx/` folder.

## Troubleshooting

### Check Logs
```bash
tail -f logs/sloth.log
```

### Run with Dry-Run First
```bash
go run . --dry-run
# Review output, then run for real
go run .
```

### Enable Verbose Output
Errors are automatically printed to stderr with stack traces.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Run tests: `make test`
4. Run linter: `make lint`
5. Submit a pull request

## License

See [LICENSE](LICENSE) file for details.

## Additional Resources

- [MODERNIZATION.md](MODERNIZATION.md) - Details on recent improvements
- [.github/copilot-instructions.md](.github/copilot-instructions.md) - AI agent guidance
