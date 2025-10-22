package main

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"sync/atomic"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
)

// AppLogger implements structured rotating logging with counters
type AppLogger struct {
	fileLogger *log.Logger
	dryRun     bool

	filesProcessed atomic.Int64
	rulesExecuted  atomic.Int64
	errorsCount    atomic.Int64
	warningsCount  atomic.Int64
}

func NewAppLogger(dryRun bool) *AppLogger {
	// Ensure directory exists (log early if it fails, but continue so program can still run)
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Printf("failed to create logs directory: %v", err)
	}

	// Set up rotatelogs
	rotator, err := rotatelogs.New(
		"logs/sloth-%Y%m%d.log",
		rotatelogs.WithLinkName("logs/sloth.log"),
		rotatelogs.WithMaxAge(30*24*time.Hour),
		rotatelogs.WithRotationTime(24*time.Hour),
	)
	if err != nil {
		log.Printf("failed to create rotatelogs: %v", err)
		// fallback to standard logger to avoid nil pointer
		return &AppLogger{fileLogger: log.Default(), dryRun: dryRun}
	}

	l := log.New(rotator, "", log.Ldate|log.Ltime|log.Lshortfile)
	return &AppLogger{fileLogger: l, dryRun: dryRun}
}

// Info logs informational messages. In normal mode: high-level only should be used at rule start/end.
// In dry-run mode it can be called for each simulated action.
func (al *AppLogger) Info(format string, args ...any) {
	al.fileLogger.Printf("INFO: "+format, args...)
}

// Warn logs warnings to file only.
func (al *AppLogger) Warn(format string, args ...any) {
	al.warningsCount.Add(1)
	al.fileLogger.Printf("WARN: "+format, args...)
}

// Error logs errors to file and also prints to stderr with stack trace for troubleshooting.
func (al *AppLogger) Error(format string, args ...any) {
	al.errorsCount.Add(1)
	msg := fmt.Sprintf(format, args...)
	al.fileLogger.Printf("ERROR: %s", msg)
	// Console output with stack trace
	fmt.Fprintf(os.Stderr, "ERROR: %s\nSTACK:\n%s\n", msg, debug.Stack())
}

// CountFile increments the files processed counter.
func (al *AppLogger) CountFile() { al.filesProcessed.Add(1) }

// CountRule increments rules executed counter.
func (al *AppLogger) CountRule() { al.rulesExecuted.Add(1) }

// Summary writes a final summary line.
func (al *AppLogger) Summary(elapsed time.Duration) {
	al.fileLogger.Printf("SUMMARY: rules=%d files=%d warnings=%d errors=%d elapsed=%.3fs dryRun=%v",
		al.rulesExecuted.Load(),
		al.filesProcessed.Load(),
		al.warningsCount.Load(),
		al.errorsCount.Load(),
		elapsed.Seconds(),
		al.dryRun,
	)
}
