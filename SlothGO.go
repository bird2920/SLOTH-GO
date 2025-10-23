package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var wg sync.WaitGroup
var readChan chan string

// dryRun indicates whether file operations should be simulated only
var dryRun bool

type folder struct {
	Name            string   `json:"name"`
	Input           string   `json:"input"`
	Output          []string `json:"output"`
	Extension       string   `json:"extension"`
	FolderType      string   `json:"folderType"`
	DeleteOlderThan int      `json:"deleteOlderThan"`
	RemoveOlderThan int      `json:"removeOlderThan,omitempty"` // legacy field retained for migration
	DryRun          bool     `json:"dryRun"`
}

func main() {
	header()

	// Parse flags early
	dryRunFlag := flag.Bool("dry-run", false, "simulate all operations without changing the filesystem")
	flag.Parse()

	// Allow env override (SLOTH_DRY_RUN=1)
	if os.Getenv("SLOTH_DRY_RUN") == "1" {
		dryRun = true
	} else {
		dryRun = *dryRunFlag
	}

	appLogger := NewAppLogger(dryRun)
	start := time.Now()
	appLogger.Info("Start time: %s", start.Format(time.RFC3339))

	balancer := &Balancer{}
	folders := getFolders(appLogger)
	elapsed := time.Since(start)

	// Use index loop to avoid implicit memory aliasing of range variable when taking its address
	for i := range folders {
		processFolder(appLogger, balancer, &folders[i])
	}

	appLogger.Summary(elapsed)
}

// deleteFiles using filepath.WalkDir (more efficient than filepath.Walk)
func deleteFiles(inPath, extension string, removeOlderThan int, appLogger *AppLogger, dryRun bool) {
	const dryRunDeleteLimit = 5
	deleteCount := 0

	e := filepath.WalkDir(inPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil {
			return err
		}

		if filepath.Ext(d.Name()) == extension && fileInfo.ModTime().Before(time.Now().AddDate(0, 0, -1*removeOlderThan)) {
			if dryRun {
				if deleteCount < dryRunDeleteLimit {
					appLogger.Info("[DRY-RUN] Would delete: %s", path)
					deleteCount++
				} else if deleteCount == dryRunDeleteLimit {
					appLogger.Info("[DRY-RUN] Reached sample limit (%d files), skipping remaining deletions", dryRunDeleteLimit)
					deleteCount++
				}
				return nil
			}
			err = os.Remove(path)
			if err != nil {
				appLogger.Error("delete failed: %v", err)
				return err
			}
			appLogger.Info("Deleted: %s", path)
		}
		return nil
	})

	if e != nil {
		appLogger.Error("delete traversal error: %v", e)
	}
}

// processFolder executes a single folder rule
func processFolder(appLogger *AppLogger, balancer *Balancer, f *folder) {
	name := f.Name
	inPath := f.Input
	outPaths := f.Output
	extension := f.Extension
	folderType := f.FolderType
	removeOlderThan := f.DeleteOlderThan
	localDryRun := dryRun || f.DryRun

	readChan = make(chan string, 100)

	if removeOlderThan > 0 && inPath != "" {
		appLogger.Info("[Rule:%s] Deleting files older than %d days from %s", name, removeOlderThan, inPath)
		deleteFiles(inPath, extension, removeOlderThan, appLogger, localDryRun)
	}

	// For delete-only rules (folderType == "delete"), skip move operations
	if strings.EqualFold(folderType, "delete") {
		appLogger.Info("[Rule:%s] Delete-only rule completed", name)
		return
	}

	files, err := os.ReadDir(inPath)
	if err != nil {
		appLogger.Error("ReadDir error: %v", err)
	}

	// Filter matching files
	var matchingFiles []string
	for _, element := range files {
		if !element.IsDir() {
			if filepath.Ext(element.Name()) == extension || extension == "" {
				matchingFiles = append(matchingFiles, element.Name())
			}
		}
	}

	// Limit dry-run to sample of 5 files to avoid massive logs
	const dryRunSampleLimit = 5
	if localDryRun && len(matchingFiles) > dryRunSampleLimit {
		appLogger.Info(
			"[Rule:%s] DRY-RUN: Found %d files, limiting to %d sample files",
			name,
			len(matchingFiles),
			dryRunSampleLimit,
		)
		matchingFiles = matchingFiles[:dryRunSampleLimit]
	}

	var numWorkers = 2 * runtime.GOMAXPROCS(0)

	appLogger.Info("[Rule:%s] Starting %d workers (dryRun=%v)", name, numWorkers, localDryRun)
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go moveFiles(appLogger, balancer, readChan, inPath, outPaths, folderType, localDryRun)
	}

	for _, fileName := range matchingFiles {
		readChan <- fileName
	}

	close(readChan)
	wg.Wait()
	appLogger.Info("[Rule:%s] Completed", name)
}

func moveFiles(
	appLogger *AppLogger,
	b *Balancer,
	inChan chan string,
	inPath string,
	outPaths []string,
	folderType string,
	localDryRun bool,
) {
	for fileToMove := range inChan {
		in := filepath.Join(inPath, fileToMove)
		balOut, err := b.Next(outPaths)
		if err != nil {
			appLogger.Error("Balancer error: %v", err)
			continue
		}
		outFolder := createOutputPath(appLogger, inPath, balOut, fileToMove, folderType)
		out := filepath.Join(outFolder, fileToMove)

		if localDryRun {
			appLogger.Info("[DRY-RUN] Would create folder: %s", outFolder)
			appLogger.Info("[DRY-RUN] Would move %s -> %s", in, out)
			continue
		}

		// Ensure destination folder exists
		if err := os.MkdirAll(outFolder, 0755); err != nil {
			appLogger.Error("mkdir failed: %v", err)
			continue
		}

		err = os.Rename(in, out)
		if err != nil {
			appLogger.Error("rename failed: %v", err)
		}
	}
	wg.Done()
}

func createOutputPath(appLogger *AppLogger, inPath, outPath, fileToMove, folderType string) string {
	fi, err := os.Stat(filepath.Join(inPath, fileToMove))
	if err != nil {
		appLogger.Error("failed to stat file %s: %v", fileToMove, err)
		return ""
	}

	mTime := fi.ModTime()

	year := strconv.Itoa(mTime.Year())
	month := strconv.Itoa(int(mTime.Month()))
	day := "Day " + strconv.Itoa(mTime.Day())

	ext := strings.SplitAfter(filepath.Ext(fi.Name()), ".")

	switch folderType {
	// 1 uses file mod time as the folder YYYY\MM\Day DD format
	case "1":
		return filepath.Join(outPath, year, month, day)

	// 2 uses the extension as the folder
	case "2":
		if len(ext) > 1 {
			return filepath.Join(outPath, ext[1])
		}
		return outPath

	// 3 uses the extension as the folder and then groups by year
	case "3":
		if len(ext) > 1 {
			return filepath.Join(outPath, ext[1], year)
		}
		return filepath.Join(outPath, year)

	// 4 will go to the root of defaultOut - ie moves files to the root of the output path
	case "4":
		return outPath

	// 5 uses mod time as the folder in YYYYMM format
	case "5":
		return filepath.Join(outPath, mTime.Format("200601"))

	default:
		return ""
	}
}

// getFolders loads config and performs migration from legacy delete rules.
func getFolders(appLogger *AppLogger) []folder {
	raw, err := os.ReadFile("config.json")
	if err != nil {
		appLogger.Error("getFolders read error: %v", err)
		os.Exit(1)
	}

	migrated, needsSave, err := migrateConfig(raw, appLogger)
	if err != nil {
		appLogger.Error("migration failed: %v", err)
		os.Exit(1)
	}

	// Write back the migrated config if changes were made
	if needsSave {
		configBytes, err := json.MarshalIndent(migrated, "", "  ")
		if err != nil {
			appLogger.Error("failed to marshal migrated config: %v", err)
		} else {
			if err := os.WriteFile("config.json", configBytes, 0600); err != nil {
				appLogger.Error("failed to write migrated config: %v", err)
			} else {
				appLogger.Info("Updated config.json with migrated settings")
			}
		}
	}

	return migrated
}

// migrateConfig updates legacy configs by converting removeOlderThan to DeleteOlderThan
// and normalizing paths. DELETE rules are kept as standalone entries (never merged).
// Returns the migrated folders, a flag indicating if the config needs to be saved, and any error.
func migrateConfig(raw []byte, appLogger *AppLogger) ([]folder, bool, error) {
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, false, err
	}

	var result []folder
	needsSave := false

	for _, m := range entries {
		f := parseFolder(m)

		// Check if legacy field exists (indicates migration needed)
		if _, hasLegacy := m["removeOlderThan"]; hasLegacy && f.RemoveOlderThan > 0 {
			needsSave = true
		}

		// Check if this is a delete rule
		if isDeleteRule(m) {
			// For delete rules, ensure folderType is normalized
			if f.FolderType == "" || strings.EqualFold(f.FolderType, "delete") {
				f.FolderType = "delete"
				needsSave = true
			}
			appLogger.Info("Migrated DELETE rule: %s (DeleteOlderThan=%d)", f.Name, f.DeleteOlderThan)
		}

		result = append(result, f)
	}

	return result, needsSave, nil
}

func isDeleteRule(m map[string]any) bool {
	if v, ok := m["folderType"].(string); ok && strings.EqualFold(v, "delete") {
		return true
	}
	if n, ok := m["name"].(string); ok && strings.Contains(strings.ToUpper(n), "DELETE") {
		return true
	}
	return false
}

func parseFolder(m map[string]any) folder {
	f := folder{}
	if v, ok := m["name"].(string); ok {
		f.Name = v
	}
	if v, ok := m["input"].(string); ok {
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
			if s, ok := o.(string); ok {
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
	// Migrate legacy removeOlderThan to new DeleteOlderThan field
	if f.DeleteOlderThan == 0 && f.RemoveOlderThan > 0 {
		f.DeleteOlderThan = f.RemoveOlderThan
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
