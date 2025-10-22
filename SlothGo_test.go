package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func outputPath(parts ...string) string {
	return filepath.Join(append([]string{string(os.PathSeparator) + "output"}, parts...)...)
}

func TestCreateOutputPath(t *testing.T) {
	// Create a temporary test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.pdf")

	// Create test file with specific mod time
	file, err := os.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()

	// Set modification time
	testTime := time.Date(2023, 10, 15, 12, 0, 0, 0, time.UTC)
	err = os.Chtimes(testFile, testTime, testTime)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		folderType string
		expected   string
	}{
		{
			name:       "FolderType 1 - Date format",
			folderType: "1",
			expected:   outputPath("2023", "10", "Day 15"),
		},
		{
			name:       "FolderType 2 - Extension",
			folderType: "2",
			expected:   outputPath("pdf"),
		},
		{
			name:       "FolderType 4 - Root",
			folderType: "4",
			expected:   outputPath(),
		},
		{
			name:       "FolderType 5 - YYYYMM",
			folderType: "5",
			expected:   outputPath("202310"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewAppLogger(true)
			result := createOutputPath(logger, tempDir, outputPath(), "test.pdf", tt.folderType)
			if result != tt.expected {
				t.Errorf("createOutputPath() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetFolders(t *testing.T) {
	// Create temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	testConfig := []folder{
		{
			Name:            "Test Rule",
			Input:           "/input/path",
			Output:          []string{"/output/path"},
			Extension:       ".pdf",
			FolderType:      "1",
			RemoveOlderThan: 30,
		},
	}

	configData, err := json.MarshalIndent(testConfig, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(configPath, configData, 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Change to temp directory
	originalDir, _ := os.Getwd()
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("Failed to restore directory: %v", err)
		}
	}()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	logger := NewAppLogger(true)
	folders := getFolders(logger)

	if len(folders) != 1 {
		t.Errorf("Expected 1 folder, got %d", len(folders))
	}

	if folders[0].Name != "Test Rule" {
		t.Errorf("Expected name 'Test Rule', got %s", folders[0].Name)
	}
}
