package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	C "strconv"
	"strings"
	"sync"
	"time"
)

var wg sync.WaitGroup
var readChan chan string
var name string
var inPath string
var outPath []string
var extension string
var folderType string

type folder struct {
	Name            string   `json:"name"`
	Input           string   `json:"input"`
	Output          []string `json:"output"`
	Extension       string   `json:"extension"`
	FolderType      string   `json:"folderType"`
	RemoveOlderThan int      `json:"removeOlderThan"`
}

func main() {
	header()

	balancer := &Balancer{}

	start := time.Now()
	log.Println("Start time:", start)

	folders := getFolders()
	elapsed := time.Since(start)

	for _, f := range folders {
		name = f.Name
		inPath = f.Input
		outPath = f.Output
		extension = f.Extension
		folderType = f.FolderType
		removeOlderThan := f.RemoveOlderThan

		readChan = make(chan string, 100)

		if folderType == "delete" {
			log.Printf("Deleting files older than %d days from %s", f.RemoveOlderThan, inPath)
			deleteFiles(inPath, extension, removeOlderThan)
		}

		files, err := ioutil.ReadDir(inPath)
		if err != nil {
			log.Println(err)
		}

		var numWorkers = 2 * runtime.GOMAXPROCS(0)

		//Start workers
		fmt.Println("Starting", numWorkers, "Workers")
		wg.Add(numWorkers)
		for i := 0; i < numWorkers; i++ {
			go moveFiles(balancer, readChan)
		}

		//Iterate over each file and move it
		for _, element := range files {
			if !element.IsDir() {
				if filepath.Ext(element.Name()) == extension || extension == "" {
					//Count number of go routines
					readChan <- element.Name()
					println(element.Name())
				}
			}
		}

		// notify readChan that no more messages are coming to avoid deadlock
		close(readChan)

		//Wait for all go routines to finish
		wg.Wait()

		elapsed = time.Since(start)
		fmt.Printf("Executed: %s\n", name)
	}

	fmt.Printf("Total execution time: %.3f seconds.", elapsed.Seconds())
}

// func delayMinute(n time.Duration) {
// 	time.Sleep(n * time.Minute)
// }

//deleteFiles using filepath.Walk
func deleteFiles(inPath, extension string, removeOlderThan int) {
	e := filepath.Walk(inPath, func(path string, file os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if file.IsDir() {
			//Remove directories that are empty
			if err := os.Remove(filepath.Dir(path)); err != nil {
				return err
			}
			log.Print("Deleted: ", filepath.Dir(path))

			return nil
		}
		if filepath.Ext(file.Name()) == extension && file.ModTime().Before(time.Now().AddDate(0, 0, -1*removeOlderThan)) {
			err = os.Remove(path)
			if err != nil {
				return err
			}
			log.Print("Deleted: ", path)

			return nil
		}
		return nil
	})

	if e != nil {
		log.Println(e)
	}
}

func moveFiles(b *Balancer, inChan chan string) {

	for fileToMove := range inChan {
		//Input file
		in := filepath.Join(inPath, fileToMove)
		balOut := b.Next(outPath)

		outFolder := createOutputPath(inPath, balOut, fileToMove)
		out := filepath.Join(outFolder, fileToMove)

		os.MkdirAll(outFolder, 0755)

		err := os.Rename(in, out)
		if err != nil {
			log.Println(err)
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

	default:
		return ""
	}
}

func getFolders() []folder {
	raw, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println("getFolders -", err.Error())
		os.Exit(1)
	}

	var c []folder
	json.Unmarshal(raw, &c)
	return c
}

func header() {
	println("Sloth: Running")
	println("----------------------")
}
