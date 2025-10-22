package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDryRunNoChanges ensures no filesystem modifications occur when dry run is enabled globally
// and per-rule, while logging still simulates actions.
func TestDryRunNoChanges(t *testing.T) {
	base := t.TempDir()
	inputDir := filepath.Join(base, "input")
	outDir := filepath.Join(base, "out")
	if err := os.MkdirAll(inputDir, 0755); err != nil { t.Fatalf("mkdir input: %v", err) }
	if err := os.MkdirAll(outDir, 0755); err != nil { t.Fatalf("mkdir out: %v", err) }

	fileA := filepath.Join(inputDir, "a.txt")
	if err := os.WriteFile(fileA, []byte("content"), 0600); err != nil { t.Fatalf("write a: %v", err) }

	// Prepare config with DryRun true at rule level; also set env for global dry-run.
	rules := []folder{
		{
			Name:       "DryMove",
			Input:      inputDir,
			Output:     []string{outDir},
			Extension:  ".txt",
			FolderType: "4",
			DryRun:     true,
		},
	}
	configBytes, err := json.MarshalIndent(rules, "", "  ")
	if err != nil { t.Fatalf("marshal: %v", err) }
	if err := os.WriteFile(filepath.Join(base, "config.json"), configBytes, 0600); err != nil { t.Fatalf("write config: %v", err) }

	// Switch working directory and set env var
	origWD, _ := os.Getwd()
	defer func(){ _ = os.Chdir(origWD); os.Unsetenv("SLOTH_DRY_RUN") }()
	if err := os.Chdir(base); err != nil { t.Fatalf("chdir: %v", err) }
	os.Setenv("SLOTH_DRY_RUN", "1")

	logger := NewAppLogger(true)
	folders := getFolders(logger)
	if len(folders) != 1 { t.Fatalf("expected 1 folder, got %d", len(folders)) }
	balancer := &Balancer{}
	for i := range folders {
		processFolder(logger, balancer, &folders[i])
	}

	// Assert input file still exists
	if _, err := os.Stat(fileA); err != nil {
		// Should not be moved
		 t.Fatalf("file unexpectedly missing in input: %v", err)
	}
	// Assert output directory does NOT contain file
	if _, err := os.Stat(filepath.Join(outDir, "a.txt")); err == nil {
		 t.Fatalf("file should not exist in output during dry-run")
	}
}
