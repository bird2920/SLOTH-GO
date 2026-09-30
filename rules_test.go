package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// inTempDir runs the test from a fresh temp directory so logs/ and stray files stay contained.
func inTempDir(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	origWD, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(base); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return base
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func assertExists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	if got := err == nil; got != want {
		t.Errorf("exists(%s) = %v, want %v", path, got, want)
	}
}

func TestMissingInputIsWarningNotError(t *testing.T) {
	base := inTempDir(t)
	out := filepath.Join(base, "Archive", "PayHeaders")
	logger := NewAppLogger(false)

	processFolder(logger, &Balancer{}, &folder{
		Name:       "Missing input",
		Input:      filepath.Join(base, "Archive", "PayHeaders"),
		Output:     []string{out},
		Extension:  ".txt",
		FolderType: "5",
	})

	if logger.Errors() != 0 || logger.Warnings() != 1 {
		t.Errorf("errors=%d warnings=%d, want 0 and 1", logger.Errors(), logger.Warnings())
	}
	assertExists(t, out, false)
}

func TestDryRunDoesNotCreateOutputDir(t *testing.T) {
	base := inTempDir(t)
	in := filepath.Join(base, "in")
	out := filepath.Join(base, "Archive")
	writeFile(t, filepath.Join(in, "a.txt"))

	processFolder(NewAppLogger(true), &Balancer{}, &folder{
		Name:            "Dry",
		Input:           in,
		Output:          []string{out},
		Extension:       ".txt",
		FolderType:      "5",
		DeleteOlderThan: 30,
		DryRun:          true,
	})

	assertExists(t, out, false)
	assertExists(t, filepath.Join(in, "a.txt"), true)
}

func TestExtensionMatchIsCaseInsensitive(t *testing.T) {
	base := inTempDir(t)
	in := filepath.Join(base, "in")
	out := filepath.Join(base, "out")
	writeFile(t, filepath.Join(in, "scan.tif"))
	writeFile(t, filepath.Join(in, "notes.txt"))

	processFolder(NewAppLogger(false), &Balancer{}, &folder{
		Name:       "Case",
		Input:      in,
		Output:     []string{out},
		Extension:  ".TIF",
		FolderType: "4",
	})

	assertExists(t, filepath.Join(out, "scan.tif"), true)
	assertExists(t, filepath.Join(in, "notes.txt"), true)
}

func TestInvalidRulesMoveNothing(t *testing.T) {
	tests := []struct {
		name       string
		folderType string
		sameOutput bool
	}{
		{name: "unknown folderType", folderType: "9"},
		{name: "empty folderType", folderType: ""},
		{name: "flat output into input", folderType: "4", sameOutput: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := inTempDir(t)
			in := filepath.Join(base, "in")
			out := filepath.Join(base, "out")
			if tt.sameOutput {
				out = in
			}
			writeFile(t, filepath.Join(in, "a.txt"))
			logger := NewAppLogger(false)

			processFolder(logger, &Balancer{}, &folder{
				Name:       tt.name,
				Input:      in,
				Output:     []string{out},
				Extension:  ".txt",
				FolderType: tt.folderType,
			})

			if logger.Errors() != 1 {
				t.Errorf("errors=%d, want 1", logger.Errors())
			}
			assertExists(t, filepath.Join(in, "a.txt"), true)
			assertExists(t, filepath.Join(base, "a.txt"), false)
		})
	}
}

func TestMigrateConfigRewritesOnlyOnce(t *testing.T) {
	legacy := []byte(`[
		{"name": "Archive DELETE", "input": "/a", "output": ["/a"], "extension": ".TIF",
		 "folderType": "delete", "deleteOlderThan": 60, "removeOlderThan": 60},
		{"name": "Old DELETE", "input": "/b", "output": ["/b"], "extension": ".csv", "removeOlderThan": 30}
	]`)
	logger := NewAppLogger(true)

	first, needsSave, err := migrateConfig(legacy, logger)
	if err != nil || !needsSave {
		t.Fatalf("first migration: needsSave=%v err=%v, want true, nil", needsSave, err)
	}
	if first[1].FolderType != folderTypeDelete || first[1].DeleteOlderThan != 30 {
		t.Errorf("legacy rule not migrated: %+v", first[1])
	}

	saved, _ := json.Marshal(first)
	if strings.Contains(string(saved), "removeOlderThan") {
		t.Errorf("saved config still contains removeOlderThan: %s", saved)
	}
	if _, needsSave, _ := migrateConfig(saved, logger); needsSave {
		t.Errorf("already-migrated config should not be rewritten")
	}
}

func writeOldFile(t *testing.T, path string, age time.Duration) {
	t.Helper()
	writeFile(t, path)
	past := time.Now().Add(-age)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func readContent(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestNameCollisionKeepsBothFiles(t *testing.T) {
	base := inTempDir(t)
	in := filepath.Join(base, "in")
	out := filepath.Join(base, "out")
	writeFile(t, filepath.Join(in, "payheader.txt"))
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"payheader.txt", "payheader_1.txt"} {
		if err := os.WriteFile(filepath.Join(out, name), []byte("archived "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	processFolder(NewAppLogger(false), &Balancer{}, &folder{
		Name: "Collide", Input: in, Output: []string{out}, Extension: ".txt", FolderType: "4",
	})

	if got := readContent(t, filepath.Join(out, "payheader.txt")); got != "archived payheader.txt" {
		t.Errorf("existing archive file was overwritten: %q", got)
	}
	if got := readContent(t, filepath.Join(out, "payheader_2.txt")); got != "x" {
		t.Errorf("moved file content = %q, want %q", got, "x")
	}
	assertExists(t, filepath.Join(in, "payheader.txt"), false)
}

func TestReserveDestinationIsUniqueUnderConcurrency(t *testing.T) {
	dir := t.TempDir()
	const n = 50
	paths := make(chan string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := reserveDestination(dir, "scan.TIF", false)
			if err != nil {
				t.Error(err)
			}
			paths <- p
		}()
	}
	wg.Wait()
	close(paths)
	seen := map[string]bool{}
	for p := range paths {
		if seen[p] {
			t.Fatalf("duplicate destination %s", p)
		}
		seen[p] = true
	}
}

func TestDeletePrunesEmptyFoldersButKeepsConfiguredOnes(t *testing.T) {
	base := inTempDir(t)
	archive := filepath.Join(base, "Archive")
	oldDay := filepath.Join(archive, "2026", "7", "Day 1")
	keptDay := filepath.Join(archive, "2026", "9", "Day 29")
	otherRuleOut := filepath.Join(archive, "PayHeaders")
	writeOldFile(t, filepath.Join(oldDay, "old.TIF"), 90*24*time.Hour)
	writeFile(t, filepath.Join(keptDay, "new.TIF"))
	if err := os.MkdirAll(otherRuleOut, 0o755); err != nil {
		t.Fatal(err)
	}

	rules := []folder{
		{Name: "Archive DELETE", Input: archive, Output: []string{archive}, Extension: ".TIF", FolderType: "delete", DeleteOlderThan: 60},
		{Name: "PayHeaders", Input: filepath.Join(base, "in"), Output: []string{otherRuleOut}, Extension: ".txt", FolderType: "5"},
	}
	configuredPaths = collectConfiguredPaths(rules)
	t.Cleanup(func() { configuredPaths = nil })

	processFolder(NewAppLogger(false), &Balancer{}, &rules[0])

	assertExists(t, filepath.Join(archive, "2026", "7"), false)
	assertExists(t, filepath.Join(keptDay, "new.TIF"), true)
	assertExists(t, otherRuleOut, true)
	assertExists(t, archive, true)
}

func TestMinAgeLeavesRecentFiles(t *testing.T) {
	base := inTempDir(t)
	in := filepath.Join(base, "in")
	out := filepath.Join(base, "out")
	writeFile(t, filepath.Join(in, "writing.txt"))
	writeOldFile(t, filepath.Join(in, "done.txt"), time.Hour)

	processFolder(NewAppLogger(false), &Balancer{}, &folder{
		Name: "MinAge", Input: in, Output: []string{out}, Extension: ".txt", FolderType: "4", MinAgeMinutes: 5,
	})

	assertExists(t, filepath.Join(in, "writing.txt"), true)
	assertExists(t, filepath.Join(out, "done.txt"), true)
}

func TestMinAgeDefault(t *testing.T) {
	if got := parseFolder(map[string]any{"name": "a"}).MinAgeMinutes; got != defaultMinAgeMinutes {
		t.Errorf("missing minAgeMinutes = %d, want default %d", got, defaultMinAgeMinutes)
	}
	if got := parseFolder(map[string]any{"minAgeMinutes": float64(0)}).MinAgeMinutes; got != 0 {
		t.Errorf("explicit minAgeMinutes 0 = %d, want 0", got)
	}
}

func TestISOWeekUsesISOYear(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	writeFile(t, file)
	dec30 := time.Date(2025, 12, 30, 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(file, dec30, dec30); err != nil {
		t.Fatal(err)
	}
	got, err := createOutputPath(dir, "out", "a.txt", "7")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("out", "2026", "Week 01"); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestForwardSlashPathsOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path semantics")
	}
	f := parseFolder(map[string]any{
		"input":  "U:/Processed File Archive/DriverPay",
		"output": []any{"//server/share/DriverPay/Archive"},
	})
	if f.Input != `U:\Processed File Archive\DriverPay` {
		t.Errorf("input = %s", f.Input)
	}
	if f.Output[0] != `\\server\share\DriverPay\Archive` {
		t.Errorf("output = %s", f.Output[0])
	}
}

func TestSavedConfigUsesForwardSlashes(t *testing.T) {
	in := []folder{{Name: "a", Input: filepath.Join("U:", "in"), Output: []string{filepath.Join("U:", "in", "Archive")}}}
	saved, _ := json.Marshal(withSlashPaths(in))
	if strings.Contains(string(saved), `\\`) {
		t.Errorf("saved config contains escaped backslashes: %s", saved)
	}
	if in[0].Output[0] != filepath.Join("U:", "in", "Archive") {
		t.Errorf("withSlashPaths modified its input")
	}
}

func TestBuildVersionPrefersStampedValue(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	version = "v1.2.3"
	if got := buildVersion(); got != "v1.2.3" {
		t.Fatalf("buildVersion() = %q, want v1.2.3", got)
	}

	version = "dev"
	if got := buildVersion(); !strings.HasPrefix(got, "dev") {
		t.Fatalf("unstamped buildVersion() = %q, want dev prefix", got)
	}
}
