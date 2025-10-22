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
	RemoveOlderThan int      `json:"removeOlderThan"`
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
	folders := getFolders()
	elapsed := time.Since(start)

	for _, f := range folders {
		processFolder(appLogger, balancer, f)
	}

	appLogger.Summary(elapsed)
}

// func delayMinute(n time.Duration) {
// 	time.Sleep(n * time.Minute)
// }

// deleteFiles using filepath.WalkDir (more efficient than filepath.Walk)
func deleteFiles(inPath, extension string, removeOlderThan int, appLogger *AppLogger) {
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
	removeOlderThan := f.RemoveOlderThan
	localDryRun := dryRun || f.DryRun

	readChan = make(chan string, 100)

	if folderType == "delete" && removeOlderThan > 0 && inPath != "" {
		appLogger.Info("[Rule:%s] Deleting files older than %d days from %s", name, removeOlderThan, inPath)
		if !localDryRun {
			deleteFiles(inPath, extension, removeOlderThan, appLogger)
		} else {
			appLogger.Info("[DRY-RUN] Would delete files older than %d days from %s", removeOlderThan, inPath)
		}
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
		outFolder := createOutputPath(inPath, balOut, fileToMove, folderType)
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

func createOutputPath(inPath string, outPath string, fileToMove string, folderType string) string {
	fi, err := os.Stat(filepath.Join(inPath, fileToMove))
	if err != nil {
		log.Println(err)
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
		outFolder = filepath.Join(outPath, ext[1])
		return outFolder

		//3 uses the extension as the folder and then groups by year
	case "3":
		outFolder = filepath.Join(outPath, ext[1], year)
		return outFolder

		//4 will go to the root of defaultOut - ie moves files to the root of the output path
	case "4":
		outFolder = filepath.Join(outPath)
		return outFolder

		//5 uses mod time as the folder in YYYYMM format
	case "5":
		outFolder = filepath.Join(outPath, mTime.Format("200601"))
		return outFolder

	case "delete":
		log.Println("folderType is delete")
		return ""

	default:
		return ""
	}
}

func getFolders() []folder {
	raw, err := os.ReadFile("config.json")
	if err != nil {
		log.Println("getFolders -", err.Error())
		os.Exit(1)
	}

	var c []folder
	json.Unmarshal(raw, &c)
	return c
}

func header() {
	log.Println("Sloth: Running")
	log.Println("----------------------")
	if dryRun {
		log.Println("DRY-RUN mode enabled: no filesystem changes will be made")
	}
}
