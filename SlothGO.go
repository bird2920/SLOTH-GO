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

	for _, f := range folders {
		processFolder(appLogger, balancer, f)
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
func processFolder(appLogger *AppLogger, balancer *Balancer, f folder) {
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
		log.Println(err)
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

		err = os.MkdirAll(outFolder, 0755)
		if err != nil {
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

func createOutputPath(appLogger *AppLogger, inPath string, outPath string, fileToMove string, folderType string) string {
	fi, err := os.Stat(filepath.Join(inPath, fileToMove))
	if err != nil {
		appLogger.Error("failed to stat file %s: %v", fileToMove, err)
		return ""
	}

	var outFolder = ""
	mTime := fi.ModTime()

	year := strconv.Itoa(mTime.Year())
	month := strconv.Itoa(int(mTime.Month()))
	day := "Day " + strconv.Itoa(mTime.Day())

	ext := strings.SplitAfter(filepath.Ext(fi.Name()), ".")

	switch folderType {

	//1 uses file mod time as the folder YYYY\MM\Day DD format
	case "1":
		outFolder = filepath.Join(outPath, year, month, day)
		return outFolder

		//2 uses the extension as the folder
	case "2":
		if len(ext) > 1 {
			outFolder = filepath.Join(outPath, ext[1])
		} else {
			outFolder = filepath.Join(outPath)
		}
		return outFolder

		//3 uses the extension as the folder and then groups by year
	case "3":
		if len(ext) > 1 {
			outFolder = filepath.Join(outPath, ext[1], year)
		} else {
			outFolder = filepath.Join(outPath, year)
		}
		return outFolder

		//4 will go to the root of defaultOut - ie moves files to the root of the output path
	case "4":
		outFolder = filepath.Join(outPath)
		return outFolder

		//5 uses mod time as the folder in YYYYMM format
	case "5":
		outFolder = filepath.Join(outPath, mTime.Format("200601"))
		return outFolder

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
// - Identifies delete configs by folderType == "delete" OR name containing "DELETE" (case-insensitive)
// - Merges removeOlderThan into matching non-delete rule (match by Input + Extension)
// - Sets DeleteOlderThan (new field) accordingly
// - Defaults DeleteOlderThan to 0 when not set
// - Logs warning if delete rule cannot be matched
// Assumption for matching: same Input and Extension uniquely identify the target rule.
func migrateConfig(raw []byte, appLogger *AppLogger) ([]folder, error) {
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}

	var result []folder
	// Index for quick lookup: key = input|extension (lower-cased)
	idx := make(map[string]*folder)
	var deleteRules []map[string]any

	isDeleteRule := func(m map[string]any) bool {
		// folderType check
		if v, ok := m["folderType"].(string); ok && strings.EqualFold(v, "delete") {
			return true
		}
		if n, ok := m["name"].(string); ok && strings.Contains(strings.ToLower(n), "delete") {
			return true
		}
		return false
	}

	// First pass: collect non-delete rules
	for _, m := range entries {
		if isDeleteRule(m) {
			deleteRules = append(deleteRules, m)
			continue
		}
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
		// Outputs
		if arr, ok := m["output"].([]any); ok {
			for _, o := range arr {
				if s, ok := o.(string); ok {
					f.Output = append(f.Output, s)
				}
			}
		}
		// Legacy fields
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
		key := strings.ToLower(f.Input + "|" + f.Extension)
		idx[key] = &f
		result = append(result, f)
	}

	// Second pass: merge delete rules
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
				// Update in both index and result slice (pointer refers to slice element copy)
				target.DeleteOlderThan = removeDays
				appLogger.Info("Migrated delete rule into '%s' (DeleteOlderThan=%d)", target.Name, removeDays)
			}
		} else {
			appLogger.Warn("Unmatched legacy delete rule for input=%s extension=%s", input, ext)
		}
	}

	return result, nil
}

func header() {
	log.Println("Sloth: Running")
	log.Println("----------------------")
	if dryRun {
		log.Println("DRY-RUN mode enabled: no filesystem changes will be made")
	}
}
