package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	C "strconv"
	"strings"
	"sync"
	"time"
)

var (
	wg         sync.WaitGroup
	readChan   chan string
	name       string
	inPath     string
	outPath    []string
	extension  string
	folderType string
)

type folder struct {
	Name       string   `json:"name"`
	Input      string   `json:"input"`
	Output     []string `json:"output"`
	Extension  string   `json:"extension"`
	FolderType string   `json:"folderType"`
}

func main() {
	header()

	balancer := &Balancer{}

	start := time.Now()
	log.Println("Start time:", start)

	folders, err := getFolders()
	if err != nil {
		log.Fatal("error getting folders", err)
	}

	elapsed := time.Since(start)

	for _, f := range folders {
		name = f.Name
		inPath = f.Input
		outPath = f.Output
		extension = f.Extension
		folderType = f.FolderType

		readChan = make(chan string, 100)

		files, err := ioutil.ReadDir(inPath)
		if err != nil {
			log.Println(err)
		}

		const numWorkers = 4

		// Start workers
		fmt.Println("Starting", numWorkers, "Workers")
		wg.Add(numWorkers)
		for i := 0; i < numWorkers; i++ {
			go moveFiles(balancer, readChan)
		}

		// Iterate over each file and move it
		for _, element := range files {
			if !element.IsDir() {
				if filepath.Ext(element.Name()) == extension || extension == "" {
					// Count number of go routines
					readChan <- element.Name()
				}
			}
		}

		// notify readChan that no more messages are coming to avoid deadlock
		close(readChan)

		// Wait for all go routines to finish
		wg.Wait()

		elapsed = time.Since(start)
		fmt.Printf("Executed: %s\n", name)
	}

	fmt.Printf("Total execution time: %.3f seconds.", elapsed.Seconds())
}

func moveFiles(b *Balancer, inChan chan string) {

	for fileToMove := range inChan {
		// Input file
		in := filepath.Join(inPath, fileToMove)
		balOut := b.Next(outPath)

		outFolder := createOutputPath(inPath, balOut, fileToMove)
		out := filepath.Join(outFolder, fileToMove)

		err := os.MkdirAll(outFolder, 0755)
		if err != nil {
			log.Printf("error making folder %s\n", outFolder)
		}

		err = os.Rename(in, out)
		if err != nil {
			log.Printf("error renaming %s to %s", in, out)
		}
	}

	wg.Done()
}

func createOutputPath(inPath string, outPath string, fileToMove string) string {
	fi, err := os.Stat(filepath.Join(inPath, fileToMove))
	if err != nil {
		log.Println(err)
	}

	var outFolder = ""
	mTime := fi.ModTime()

	year := C.Itoa(mTime.Year())
	month := C.Itoa(int(mTime.Month()))
	day := "Day " + C.Itoa(mTime.Day())

	ext := strings.SplitAfter(filepath.Ext(fi.Name()), ".")

	switch folderType {

	// 1 uses file mod time as the folder YYYY\MM\Day DD format
	case "1":
		outFolder = filepath.Join(outPath, year, month, day)
		return outFolder

	// 2 uses the extension as the folder
	case "2":
		outFolder = filepath.Join(outPath, ext[1])
		return outFolder

	// 3 uses the extension as the folder and then groups by year
	case "3":
		outFolder = filepath.Join(outPath, ext[1], year)
		return outFolder

	// 4 will go to the root of defaultOut - ie moves files to the root of the output path
	case "4":
		outFolder = filepath.Join(outPath)
		return outFolder

	// 5 uses mod time as the folder in YYYYMM format
	case "5":
		outFolder = filepath.Join(outPath, mTime.Format("200601"))
		return outFolder

	default:
		return ""
	}
}

func getFolders() ([]folder, error) {
	raw, err := ioutil.ReadFile("config.json")
	if err != nil {
		return nil, err
	}

	var c []folder
	err = json.Unmarshal(raw, &c)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func header() {
	println("Sloth: Running")
	println("----------------------")
}
