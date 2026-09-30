package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var folderTypeDescriptions = map[string]string{
	"1": "By Mod Time (YYYY/MM/Day DD)",
	"2": "By Extension",
	"3": "By Extension + Year",
	"4": "Flat Output (root only)",
	"5": "Compact Date (YYYYMM)",
	"6": "Standard Date (YYYY/MM/DD)",
	"7": "ISO Week (YYYY/Week NN)",
}

const (
	folderTypeDelete = "delete"

	// defaultMinAgeMinutes leaves recently modified files for the next run so files
	// still being written are not moved. Set "minAgeMinutes": 0 on a rule to disable.
	defaultMinAgeMinutes = 5

	maxCollisionSuffix = 1000
)

func listFolderTypes() {
	keys := make([]string, 0, len(folderTypeDescriptions))
	for k := range folderTypeDescriptions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println("Available Folder Types:")
	for _, k := range keys {
		fmt.Printf("  %s: %s\n", k, folderTypeDescriptions[k])
	}
	fmt.Printf("  %s: Delete only (no move)\n", folderTypeDelete)
}

// dryRun indicates whether file operations should be simulated only
var dryRun bool

// configuredPaths holds every rule's input and output folder (lowercased) so empty-folder
// cleanup never removes a folder the config depends on.
var configuredPaths map[string]bool

type folder struct {
	Name            string   `json:"name"`
	Input           string   `json:"input"`
	Output          []string `json:"output"`
	Extension       string   `json:"extension"`
	FolderType      string   `json:"folderType"`
	DeleteOlderThan int      `json:"deleteOlderThan"`
	MinAgeMinutes   int      `json:"minAgeMinutes"`
	RemoveOlderThan int      `json:"removeOlderThan,omitempty"` // legacy field retained for migration
	DryRun          bool     `json:"dryRun"`
}

func main() {
	dryRunFlag := flag.Bool("dry-run", false, "simulate all operations without changing the filesystem")
	showTypes := flag.Bool("list-types", false, "Show available folder type options")
	showVersion := flag.Bool("version", false, "Print the build version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildVersion())
		os.Exit(0)
	}
	if *showTypes {
		listFolderTypes()
		os.Exit(0)
	}

	// Allow env override (SLOTH_DRY_RUN=1)
	dryRun = *dryRunFlag || os.Getenv("SLOTH_DRY_RUN") == "1"
	header()

	appLogger := NewAppLogger(dryRun)
	start := time.Now()
	appLogger.Info("Version: %s", buildVersion())
	appLogger.Info("Start time: %s", start.Format(time.RFC3339))

	balancer := &Balancer{}
	folders := getFolders(appLogger)
	configuredPaths = collectConfiguredPaths(folders)

	// Use index loop to avoid implicit memory aliasing of range variable when taking its address
	for i := range folders {
		processFolder(appLogger, balancer, &folders[i])
	}

	elapsed := time.Since(start)
	appLogger.Summary(elapsed)

	log.Printf("Sloth: Completed in %s (dryRun=%v, warnings=%d, errors=%d). See detailed log: %s",
		elapsed.Truncate(time.Millisecond), dryRun, appLogger.Warnings(), appLogger.Errors(), "logs/sloth.log")
}

// processFolder executes a single folder rule
func processFolder(appLogger *AppLogger, balancer *Balancer, f *folder) {
	appLogger.CountRule()
	localDryRun := dryRun || f.DryRun

	if err := validateRule(f); err != nil {
		appLogger.Error("[Rule:%s] Invalid rule, skipping: %v", f.Name, err)
		return
	}
	if !dirExists(f.Input) {
		appLogger.Warn("[Rule:%s] Input folder not found, skipping: %s", f.Name, f.Input)
		return
	}

	if isDeleteOnly(f.FolderType) {
		appLogger.Info("[Rule:%s] Deleting files older than %d days from INPUT: %s", f.Name, f.DeleteOlderThan, f.Input)
		deleteFiles(appLogger, f.Input, f.Extension, f.DeleteOlderThan, localDryRun)
		appLogger.Info("[Rule:%s] Completed", f.Name)
		return
	}

	if err := ensureOutputDirs(appLogger, f.Name, f.Output, localDryRun); err != nil {
		return
	}

	minAge := time.Duration(f.MinAgeMinutes) * time.Minute
	files, tooNew, err := collectMatchingFiles(f.Input, f.Extension, minAge)
	if err != nil {
		appLogger.Error("[Rule:%s] Failed to read input folder %s: %v", f.Name, f.Input, err)
		return
	}
	if tooNew > 0 {
		appLogger.Info("[Rule:%s] Leaving %d files modified in the last %d minutes for the next run", f.Name, tooNew, f.MinAgeMinutes)
	}
	files = applyDryRunSampleLimit(appLogger, f.Name, files, localDryRun)
	runMoveWorkers(appLogger, balancer, f, localDryRun, files)

	if f.DeleteOlderThan > 0 {
		appLogger.Info("[Rule:%s] Deleting files older than %d days from OUTPUT paths", f.Name, f.DeleteOlderThan)
		for _, outPath := range f.Output {
			deleteFiles(appLogger, outPath, f.Extension, f.DeleteOlderThan, localDryRun)
		}
	}
	appLogger.Info("[Rule:%s] Completed", f.Name)
}

// validateRule catches config mistakes before any file is touched.
func validateRule(f *folder) error {
	if f.Input == "" {
		return errors.New("input is empty")
	}
	if isDeleteOnly(f.FolderType) {
		if f.DeleteOlderThan <= 0 {
			return errors.New("delete rule needs deleteOlderThan > 0")
		}
		return nil
	}
	if _, ok := folderTypeDescriptions[f.FolderType]; !ok {
		return fmt.Errorf("unknown folderType %q (run with -list-types)", f.FolderType)
	}
	if len(f.Output) == 0 {
		return errors.New("output is empty")
	}
	for _, out := range f.Output {
		if f.FolderType == "4" && samePath(out, f.Input) {
			return fmt.Errorf("output %s is the same as input with folderType 4 (nothing would move)", out)
		}
	}
	return nil
}

func isDeleteOnly(folderType string) bool { return strings.EqualFold(folderType, folderTypeDelete) }

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// samePath compares paths case-insensitively, matching Windows filesystem semantics.
func samePath(a, b string) bool { return pathKey(a) == pathKey(b) }

// matchesExtension compares case-insensitively so ".TIF" matches "scan.tif". An empty extension matches everything.
func matchesExtension(name, extension string) bool {
	return extension == "" || strings.EqualFold(filepath.Ext(name), extension)
}

// ensureOutputDirs creates each output folder if its parent exists. In dry-run it only reports.
func ensureOutputDirs(appLogger *AppLogger, name string, outPaths []string, localDryRun bool) error {
	for _, outPath := range outPaths {
		if dirExists(outPath) {
			continue
		}
		parentDir := filepath.Dir(outPath)
		if !dirExists(parentDir) {
			err := fmt.Errorf("output parent folder does not exist: %s", parentDir)
			appLogger.Error("[Rule:%s] %v (cannot auto-create)", name, err)
			return err
		}
		if localDryRun {
			appLogger.Info("[DRY-RUN] [Rule:%s] Would create output folder: %s", name, outPath)
			continue
		}
		if err := os.Mkdir(outPath, 0o755); err != nil {
			appLogger.Error("[Rule:%s] Failed to create output folder %s: %v", name, outPath, err)
			return err
		}
		appLogger.Info("[Rule:%s] Created output folder: %s", name, outPath)
	}
	return nil
}

// collectMatchingFiles lists matching files directly inside inPath (not recursive).
// Files modified within minAge are left out and counted in tooNew.
func collectMatchingFiles(inPath, extension string, minAge time.Duration) (matching []string, tooNew int, err error) {
	entries, err := os.ReadDir(inPath)
	if err != nil {
		return nil, 0, err
	}
	cutoff := time.Now().Add(-minAge)
	for _, e := range entries {
		if e.IsDir() || !matchesExtension(e.Name(), extension) {
			continue
		}
		if minAge > 0 {
			info, err := e.Info()
			if err != nil || info.ModTime().After(cutoff) {
				tooNew++
				continue
			}
		}
		matching = append(matching, e.Name())
	}
	return matching, tooNew, nil
}

func applyDryRunSampleLimit(appLogger *AppLogger, name string, files []string, localDryRun bool) []string {
	const dryRunSampleLimit = 5
	if localDryRun && len(files) > dryRunSampleLimit {
		appLogger.Info("[Rule:%s] DRY-RUN: Found %d files, limiting to %d sample files", name, len(files), dryRunSampleLimit)
		return files[:dryRunSampleLimit]
	}
	return files
}

func runMoveWorkers(appLogger *AppLogger, balancer *Balancer, f *folder, localDryRun bool, files []string) {
	if len(files) == 0 {
		appLogger.Info("[Rule:%s] No matching files found", f.Name)
		return
	}
	numWorkers := min(2*runtime.GOMAXPROCS(0), len(files))
	appLogger.Info("[Rule:%s] Moving %d files with %d workers (dryRun=%v)", f.Name, len(files), numWorkers, localDryRun)

	jobs := make(chan string, len(files))
	for _, fn := range files {
		jobs <- fn
	}
	close(jobs)

	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for fileName := range jobs {
				moveFile(appLogger, balancer, f, fileName, localDryRun)
			}
		}()
	}
	wg.Wait()
}

func moveFile(appLogger *AppLogger, b *Balancer, f *folder, fileName string, localDryRun bool) {
	in := filepath.Join(f.Input, fileName)
	balOut, err := b.Next(f.Output)
	if err != nil {
		appLogger.Error("[Rule:%s] Balancer error: %v", f.Name, err)
		return
	}
	outFolder, err := createOutputPath(f.Input, balOut, fileName, f.FolderType)
	if err != nil {
		appLogger.Error("[Rule:%s] Skipping %s: %v", f.Name, in, err)
		return
	}
	if localDryRun {
		out, err := reserveDestination(outFolder, fileName, true)
		if err != nil {
			appLogger.Error("[Rule:%s] Skipping %s: %v", f.Name, in, err)
			return
		}
		appLogger.Info("[DRY-RUN] Would move %s -> %s", in, out)
		appLogger.CountFile()
		return
	}
	if err := os.MkdirAll(outFolder, 0o755); err != nil {
		appLogger.Error("[Rule:%s] Failed to create folder %s: %v", f.Name, outFolder, err)
		return
	}
	out, err := reserveDestination(outFolder, fileName, false)
	if err != nil {
		appLogger.Error("[Rule:%s] Skipping %s: %v", f.Name, in, err)
		return
	}
	if err := os.Rename(in, out); err != nil {
		_ = os.Remove(out) // drop the empty placeholder
		appLogger.Error("[Rule:%s] Move failed: %v", f.Name, err)
		return
	}
	if filepath.Base(out) != fileName {
		appLogger.Info("[Rule:%s] %s already existed in archive, saved as %s", f.Name, fileName, filepath.Base(out))
	}
	appLogger.CountFile()
}

// reserveDestination returns a path in dir that does not exist yet, adding _1, _2, ... before
// the extension when fileName is taken. os.Rename replaces existing files on Windows, so this
// prevents archived copies being overwritten. Outside dry-run it creates an empty placeholder
// so concurrent workers can never pick the same name; the rename then replaces the placeholder.
func reserveDestination(dir, fileName string, localDryRun bool) (string, error) {
	ext := filepath.Ext(fileName)
	stem := strings.TrimSuffix(fileName, ext)
	for i := 0; i < maxCollisionSuffix; i++ {
		name := fileName
		if i > 0 {
			name = fmt.Sprintf("%s_%d%s", stem, i, ext)
		}
		path := filepath.Join(dir, name)
		if localDryRun {
			if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
				return path, nil
			}
			continue
		}
		placeholder, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			return path, placeholder.Close()
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("no free name for %s in %s after %d attempts", fileName, dir, maxCollisionSuffix)
}

// createOutputPath returns the destination folder for a file based on folderType.
func createOutputPath(inPath, outPath, fileName, folderType string) (string, error) {
	fi, err := os.Stat(filepath.Join(inPath, fileName))
	if err != nil {
		return "", err
	}

	mTime := fi.ModTime()
	year := strconv.Itoa(mTime.Year())
	ext := strings.TrimPrefix(filepath.Ext(fi.Name()), ".")

	switch folderType {
	case "1": // YYYY\M\Day D from mod time
		return filepath.Join(outPath, year, strconv.Itoa(int(mTime.Month())), "Day "+strconv.Itoa(mTime.Day())), nil
	case "2": // by extension
		return filepath.Join(outPath, ext), nil
	case "3": // by extension, then year
		return filepath.Join(outPath, ext, year), nil
	case "4": // root of output
		return outPath, nil
	case "5": // YYYYMM
		return filepath.Join(outPath, mTime.Format("200601")), nil
	case "6": // YYYY/MM/DD
		return filepath.Join(outPath, mTime.Format("2006/01/02")), nil
	case "7": // ISO year/Week NN (Dec 30 can be week 1 of the next year)
		isoYear, week := mTime.ISOWeek()
		return filepath.Join(outPath, strconv.Itoa(isoYear), fmt.Sprintf("Week %02d", week)), nil
	default:
		return "", fmt.Errorf("unknown folderType %q", folderType)
	}
}

// deleteFiles removes matching files older than olderThanDays anywhere under root, then
// removes any empty folders left below root. Individual failures are logged and the walk continues.
func deleteFiles(appLogger *AppLogger, root, extension string, olderThanDays int, localDryRun bool) {
	const dryRunDeleteLimit = 5
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	count := 0
	var dirs []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		switch {
		case err != nil:
			if path == root {
				return err
			}
			appLogger.Warn("Cannot read %s: %v", path, err)
		case d.IsDir():
			if path != root {
				dirs = append(dirs, path)
			}
		case isExpired(appLogger, path, d, extension, cutoff):
			count++
			removeExpired(appLogger, path, localDryRun, count <= dryRunDeleteLimit)
		}
		return nil
	})

	switch {
	case errors.Is(err, os.ErrNotExist) && localDryRun:
		// Output folder not created yet during a dry run; nothing to delete.
	case err != nil:
		appLogger.Error("Delete scan of %s failed: %v", root, err)
	case localDryRun && count > dryRunDeleteLimit:
		appLogger.Info("[DRY-RUN] ...and %d more files would be deleted from %s", count-dryRunDeleteLimit, root)
	}
	if err == nil {
		pruneEmptyDirs(appLogger, root, dirs, localDryRun)
	}
}

// isExpired reports whether the file matches extension and was last modified before cutoff.
func isExpired(appLogger *AppLogger, path string, d os.DirEntry, extension string, cutoff time.Time) bool {
	if !matchesExtension(d.Name(), extension) {
		return false
	}
	info, err := d.Info()
	if err != nil {
		appLogger.Warn("Cannot stat %s: %v", path, err)
		return false
	}
	return info.ModTime().Before(cutoff)
}

// removeExpired deletes one expired file, or in dry-run logs it when logDryRun is set.
func removeExpired(appLogger *AppLogger, path string, localDryRun, logDryRun bool) {
	if localDryRun {
		if logDryRun {
			appLogger.Info("[DRY-RUN] Would delete: %s", path)
		}
		return
	}
	if err := os.Remove(path); err != nil {
		appLogger.Error("Delete failed: %v", err)
		return
	}
	appLogger.Info("Deleted: %s", path)
}

// pruneEmptyDirs removes empty folders from dirs (all below root, in walk order), deepest first
// so emptied parents go too. Folders used by any rule are kept.
func pruneEmptyDirs(appLogger *AppLogger, root string, dirs []string, localDryRun bool) {
	removed := 0
	for i := len(dirs) - 1; i >= 0; i-- {
		dir := dirs[i]
		if configuredPaths[pathKey(dir)] {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			continue
		}
		if !localDryRun {
			if err := os.Remove(dir); err != nil {
				appLogger.Warn("Cannot remove empty folder %s: %v", dir, err)
				continue
			}
		}
		removed++
	}
	if removed == 0 {
		return
	}
	if localDryRun {
		appLogger.Info("[DRY-RUN] Would remove %d empty folders under %s", removed, root)
	} else {
		appLogger.Info("Removed %d empty folders under %s", removed, root)
	}
}

// collectConfiguredPaths gathers every rule's input and output folder.
func collectConfiguredPaths(folders []folder) map[string]bool {
	paths := make(map[string]bool)
	for i := range folders {
		paths[pathKey(folders[i].Input)] = true
		for _, out := range folders[i].Output {
			paths[pathKey(out)] = true
		}
	}
	return paths
}

// pathKey normalizes a path for case-insensitive (Windows) comparison.
func pathKey(p string) string { return strings.ToLower(filepath.Clean(p)) }

// getFolders loads config and performs migration from legacy delete rules.
func getFolders(appLogger *AppLogger) []folder {
	raw, err := os.ReadFile("config.json")
	if err != nil {
		appLogger.Error("Cannot read config.json: %v", err)
		os.Exit(1)
	}

	migrated, needsSave, err := migrateConfig(raw, appLogger)
	if err != nil {
		appLogger.Error("Cannot parse config.json: %v", err)
		os.Exit(1)
	}

	if needsSave && dryRun {
		appLogger.Info("[DRY-RUN] Would update config.json with migrated settings")
	} else if needsSave {
		configBytes, err := json.MarshalIndent(withSlashPaths(migrated), "", "  ")
		if err != nil {
			appLogger.Error("Failed to marshal migrated config: %v", err)
		} else if err := os.WriteFile("config.json", configBytes, 0o600); err != nil {
			appLogger.Error("Failed to write migrated config: %v", err)
		} else {
			appLogger.Info("Updated config.json with migrated settings")
		}
	}

	return migrated
}

// migrateConfig converts legacy removeOlderThan to deleteOlderThan and normalizes delete rules.
// DELETE rules are kept as standalone entries (never merged).
// Returns the migrated folders and whether the config file should be rewritten.
func migrateConfig(raw []byte, appLogger *AppLogger) ([]folder, bool, error) {
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, false, err
	}

	result := make([]folder, 0, len(entries))
	needsSave := false

	for _, m := range entries {
		f := parseFolder(m)

		if f.RemoveOlderThan > 0 {
			if f.DeleteOlderThan == 0 {
				f.DeleteOlderThan = f.RemoveOlderThan
			}
			f.RemoveOlderThan = 0
			needsSave = true
		}

		// Legacy delete rules were identified by "DELETE" in the name with no folderType.
		legacyDelete := f.FolderType == "" && strings.Contains(strings.ToUpper(f.Name), "DELETE")
		if legacyDelete || (isDeleteOnly(f.FolderType) && f.FolderType != folderTypeDelete) {
			f.FolderType = folderTypeDelete
			needsSave = true
			appLogger.Info("Migrated DELETE rule: %s (DeleteOlderThan=%d)", f.Name, f.DeleteOlderThan)
		}

		result = append(result, f)
	}

	return result, needsSave, nil
}

// withSlashPaths returns a copy of folders with forward-slash paths for writing config.json,
// so saved configs never need escaped backslashes. Paths are converted back to the OS
// separator by filepath.Clean when the config is loaded.
func withSlashPaths(folders []folder) []folder {
	out := make([]folder, len(folders))
	for i, f := range folders {
		f.Input = filepath.ToSlash(f.Input)
		f.Output = make([]string, len(folders[i].Output))
		for j, o := range folders[i].Output {
			f.Output[j] = filepath.ToSlash(o)
		}
		out[i] = f
	}
	return out
}

func parseFolder(m map[string]any) folder {
	f := folder{}
	if v, ok := m["name"].(string); ok {
		f.Name = v
	}
	if v, ok := m["input"].(string); ok && v != "" {
		f.Input = filepath.Clean(v)
	}
	if v, ok := m["extension"].(string); ok {
		f.Extension = v
	}
	if v, ok := m["folderType"].(string); ok {
		f.FolderType = v
	}
	if v, ok := m["dryRun"].(bool); ok {
		f.DryRun = v
	}
	if arr, ok := m["output"].([]any); ok {
		for _, o := range arr {
			if s, ok := o.(string); ok && s != "" {
				f.Output = append(f.Output, filepath.Clean(s))
			}
		}
	}
	if v, ok := m["removeOlderThan"].(float64); ok {
		f.RemoveOlderThan = int(v)
	}
	if v, ok := m["deleteOlderThan"].(float64); ok {
		f.DeleteOlderThan = int(v)
	}
	f.MinAgeMinutes = defaultMinAgeMinutes
	if v, ok := m["minAgeMinutes"].(float64); ok {
		f.MinAgeMinutes = int(v)
	}
	return f
}

func header() {
	log.Println("Sloth: Running")
	log.Println("----------------------")
	if dryRun {
		log.Println("DRY-RUN mode enabled: no filesystem changes will be made")
	}
}
