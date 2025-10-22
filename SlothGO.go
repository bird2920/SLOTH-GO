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
				appLogger.Info("[DRY-RUN] Would delete: %s", path)
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

	files, err := os.ReadDir(inPath)
	if err != nil {
		appLogger.Error("ReadDir error: %v", err)
	}

	var numWorkers = 2 * runtime.GOMAXPROCS(0)

	appLogger.Info("[Rule:%s] Starting %d workers (dryRun=%v)", name, numWorkers, localDryRun)
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go moveFiles(appLogger, balancer, readChan, inPath, outPaths, folderType, localDryRun)
	}

	for _, element := range files {
		if !element.IsDir() {
			if filepath.Ext(element.Name()) == extension || extension == "" {
				readChan <- element.Name()
			}
		}
	}

	close(readChan)
	wg.Wait()
	appLogger.Info("[Rule:%s] Completed", name)
}

func moveFiles(appLogger *AppLogger, b *Balancer, inChan chan string, inPath string, outPaths []string, folderType string, localDryRun bool) {
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

	migrated, err := migrateConfig(raw, appLogger)
	if err != nil {
		appLogger.Error("migration failed: %v", err)
		os.Exit(1)
	}
	return migrated
}

// migrateConfig updates legacy configs:
// - Identifies delete configs by folderType == "delete" OR name containing "DELETE"
// - Merges removeOlderThan into matching non-delete rule (match by Input + Extension)
// - Sets DeleteOlderThan (new field) accordingly
// - Defaults DeleteOlderThan to 0 when not set
// - Logs warning if delete rule cannot be matched
func migrateConfig(raw []byte, appLogger *AppLogger) ([]folder, error) {
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}

	var result []folder
	idx := make(map[string]*folder)
	var deleteRules []map[string]any

	// First pass: collect non-delete rules
	for _, m := range entries {
		if isDeleteRule(m) {
			deleteRules = append(deleteRules, m)
			continue
		}
		f := parseFolder(m)
		key := strings.ToLower(f.Input + "|" + f.Extension)
		idx[key] = &f
		result = append(result, f)
	}

	// Second pass: merge delete rules
	result = mergeDeleteRules(deleteRules, idx, result, appLogger)

	return result, nil
}

func isDeleteRule(m map[string]any) bool {
	if v, ok := m["folderType"].(string); ok && strings.EqualFold(v, "delete") {
		return true
	}
	if n, ok := m["name"].(string); ok && strings.Contains(strings.ToLower(n), "delete") {
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
		f.Input = v
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
				f.Output = append(f.Output, s)
			}
		}
	}
	if v, ok := m["removeOlderThan"].(float64); ok {
		f.RemoveOlderThan = int(v)
	}
	if v, ok := m["deleteOlderThan"].(float64); ok {
		f.DeleteOlderThan = int(v)
	}
	// Default new field from legacy if present and not already set
	if f.DeleteOlderThan == 0 && f.RemoveOlderThan > 0 {
		f.DeleteOlderThan = f.RemoveOlderThan
	}
	return f
}

func mergeDeleteRules(
	deleteRules []map[string]any,
	idx map[string]*folder,
	result []folder,
	logger *AppLogger,
) []folder {
	for _, dr := range deleteRules {
		input := ""
		ext := ""
		if v, ok := dr["input"].(string); ok {
			input = v
		}
		if v, ok := dr["extension"].(string); ok {
			ext = v
		}
		removeDays := 0
		if v, ok := dr["removeOlderThan"].(float64); ok {
			removeDays = int(v)
		}
		if v, ok := dr["deleteOlderThan"].(float64); ok {
			removeDays = int(v)
		}

		key := strings.ToLower(input + "|" + ext)
		if target, exists := idx[key]; exists {
			if removeDays > 0 {
				target.DeleteOlderThan = removeDays
				logger.Info(
					"Migrated delete rule into '%s' (DeleteOlderThan=%d)",
					target.Name,
					removeDays,
				)
			}
		} else {
			f := convertDeleteRule(dr, input, ext, removeDays)
			result = append(result, f)
			msg := "Unmatched legacy delete rule for input=%s extension=%s" +
				" converted to normal folder"
			logger.Warn(msg, input, ext)
		}
	}
	return result
}

func convertDeleteRule(
	dr map[string]any,
	input, ext string,
	removeDays int,
) folder {
	f := folder{}
	if v, ok := dr["name"].(string); ok {
		name := strings.ReplaceAll(v, "DELETE", "")
		name = strings.TrimSpace(name)
		f.Name = name
	}
	f.Input = input
	f.Extension = ext
	f.FolderType = "1"
	f.DeleteOlderThan = removeDays
	if v, ok := dr["dryRun"].(bool); ok {
		f.DryRun = v
	}
	if arr, ok := dr["output"].([]any); ok {
		for _, o := range arr {
			if s, ok := o.(string); ok {
				f.Output = append(f.Output, s)
			}
		}
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
