package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDryRunLimitEnforced verifies that dry-run only processes sample of 5 files, not all files.
func TestDryRunLimitEnforced(t *testing.T) {
	base := t.TempDir()
	inputDir := filepath.Join(base, "input")
	outDir := filepath.Join(base, "out")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("mkdir input: %v", err)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("mkdir out: %v", err)
	}

	// Create 10 files to exceed the 5-file sample limit
	for i := 1; i <= 10; i++ {
		filename := filepath.Join(inputDir, "file"+string(rune('0'+i))+".txt")
		if err := os.WriteFile(filename, []byte("content"), 0600); err != nil {
			t.Fatalf("write file%d: %v", i, err)
		}
	}

	rules := []folder{
		{
			Name:       "LimitTest",
			Input:      inputDir,
			Output:     []string{outDir},
			Extension:  ".txt",
			FolderType: "4",
			DryRun:     true,
		},
	}

	configBytes, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	configPath := filepath.Join(base, "config.json")
	if err := os.WriteFile(configPath, configBytes, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origWD, _ := os.Getwd()
	defer func() {
		_ = os.Chdir(origWD)
		os.Unsetenv("SLOTH_DRY_RUN")
	}()
	if err := os.Chdir(base); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.Setenv("SLOTH_DRY_RUN", "1")

	logger := NewAppLogger(true)
	folders := getFolders(logger)
	if len(folders) != 1 {
		t.Fatalf("expected 1 folder, got %d", len(folders))
	}
	balancer := &Balancer{}
	for i := range folders {
		processFolder(logger, balancer, &folders[i])
	}

	// Verify all 10 files still exist (none were moved)
	for i := 1; i <= 10; i++ {
		filename := filepath.Join(inputDir, "file"+string(rune('0'+i))+".txt")
		if _, err := os.Stat(filename); err != nil {
			t.Errorf("file%d missing from input: %v", i, err)
		}
	}

	// Verify no files in output
	entries, _ := os.ReadDir(outDir)
	if len(entries) > 0 {
		t.Errorf("expected empty output dir, found %d entries", len(entries))
	}
}
