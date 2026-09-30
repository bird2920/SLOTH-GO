package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestIntegrationRealMoveDelete exercises moving files (folderType 1 & 4) and deleting old files.
func TestIntegrationRealMoveDelete(t *testing.T) {
	t.TempDir() // ensure parallel-safe cleanup
	base := t.TempDir()
	inputDir := filepath.Join(base, "input")
	outputDir := filepath.Join(base, "out")
	outputDir2 := filepath.Join(base, "out2")

	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatalf("failed to mkdir input: %v", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("failed to mkdir output: %v", err)
	}
	if err := os.MkdirAll(outputDir2, 0755); err != nil {
		t.Fatalf("failed to mkdir output2: %v", err)
	}

	// Create fresh file and old file (to be deleted)
	freshTxt := filepath.Join(inputDir, "fresh.txt")
	freshLog := filepath.Join(inputDir, "fresh.log")
	oldFile := filepath.Join(inputDir, "old.txt")
	if err := os.WriteFile(freshTxt, []byte("fresh"), 0600); err != nil {
		t.Fatalf("write fresh txt: %v", err)
	}
	if err := os.WriteFile(freshLog, []byte("fresh"), 0600); err != nil {
		t.Fatalf("write fresh log: %v", err)
	}
	if err := os.WriteFile(oldFile, []byte("old"), 0600); err != nil {
		t.Fatalf("write old: %v", err)
	}

	// Age the old file by setting its mtime 2 days in past
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, past, past); err != nil {
		t.Fatalf("chtimes old: %v", err)
	}

	// Build config: one rule that deletes .txt older than 1 day & moves files using folderType 4 (root) to two outputs (balancer)
	// second rule moves by date (folderType 1) without deletion.
	rules := []folder{
		{
			Name:            "DeleteAndMoveRoot",
			Input:           inputDir,
			Output:          []string{outputDir, outputDir2},
			Extension:       ".txt",
			FolderType:      "4",
			DeleteOlderThan: 1,
		},
		{
			Name:       "DateMove",
			Input:      inputDir,
			Output:     []string{outputDir},
			Extension:  ".log",
			FolderType: "1",
		},
	}

	configBytes, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	// Write config.json in test working directory (change CWD to base)
	if err := os.WriteFile(filepath.Join(base, "config.json"), configBytes, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	origWD, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWD) }()
	if err := os.Chdir(base); err != nil {
		t.Fatalf("chdir base: %v", err)
	}

	// Run main logic pieces: getFolders then execute rules manually to avoid exiting.
	logger := NewAppLogger(false)
	balancer := &Balancer{}
	folders := getFolders(logger)
	if len(folders) != 2 {
		t.Fatalf("expected 2 folders, got %d", len(folders))
	}

	for i := range folders {
		processFolder(logger, balancer, &folders[i])
	}

	// Validate: old file deleted, fresh file moved to one of output roots (folderType 4) and also date-based folder tree.
	if _, err := os.Stat(oldFile); err == nil {
		// Should be deleted
		t.Fatalf("old file not deleted")
	}
	// Fresh file should no longer be in input
	if _, err := os.Stat(freshTxt); err == nil {
		t.Fatalf("fresh file still present in input - move did not occur")
	}

	// Check that file exists in one of output roots or date folders
	foundMoved := false
	// root move (folderType 4) either in outputDir or outputDir2
	if _, err := os.Stat(filepath.Join(outputDir, "fresh.txt")); err == nil {
		foundMoved = true
	}
	if _, err := os.Stat(filepath.Join(outputDir2, "fresh.txt")); err == nil {
		foundMoved = true
	}
	// date move (folderType 1) -> nested structure (YYYY/MM/Day DD/)
	year := time.Now().Format("2006")
	month := time.Now().Format("01")
	dayDir := "Day " + time.Now().Format("02")
	datePath := filepath.Join(outputDir, year, month, dayDir, "fresh.log")
	if _, err := os.Stat(datePath); err == nil {
		foundMoved = true
	}
	if !foundMoved {
		// Provide visibility into directory tree on failure
		_ = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				// Print discovered files for debugging
				t.Logf("found file: %s", path)
			}
			return nil
		})
		_ = filepath.Walk(outputDir2, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				t.Logf("found file: %s", path)
			}
			return nil
		})
		// Fail with concise message
		t.Fatalf("fresh file not found in any expected output location")
	}
}
