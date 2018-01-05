package main

import (
	N "IFTTT/notify"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	C "strconv"
	"sync"
	"time"
)

var wg sync.WaitGroup
var readChan chan string
var name string
var inPath string
var outPath string
var pattern string
var folderType string

type folder struct {
	Name       string `json:"name"`
	Input      string `json:"input"`
	Output     string `json:"output"`
	Pattern    string `json:"pattern"`
	FolderType string `json:"folderType"`
}

func main() {
	header()

	start := time.Now()
	log.Println("Start time:", start)

	folders := getFolders()

	for _, f := range folders {
		name = f.Name
		inPath = f.Input
		outPath = f.Output
		pattern = f.Pattern
		folderType = f.FolderType

		//fmt.Println(name)
		//fmt.Println(inPath, outPath, Pattern, folderType)

		readChan = make(chan string, 100)

		files, _ := ioutil.ReadDir(inPath)
		if err := recover(); err != nil {
			log.Println(err)
		}

		//Start workers
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go moveFiles(readChan)
		}

		//Iterate over each file and move it
		for _, element := range files {
			if !element.IsDir() {
				if string([]byte(element.Name())[len(element.Name())-len(pattern):]) == pattern {
					//Count number of go routines
					readChan <- element.Name()
					//println(element.Name())
				}
			}
		}

		close(readChan)

		//Wait for all go routines to finish
		wg.Wait()

		N.Notify("https://maker.ifttt.com/trigger/Sloth_Notify/with/key/bhhXR_IRBjXQxQPOgI0Q7b", "application/json", name, outPath, pattern)

		elapsed := time.Since(start)
		fmt.Printf("Execution Time: %.2f seconds to run %s\n", elapsed.Seconds(), name)
	}
}

func moveFiles(inChan chan string) {

	for fileToMove := range inChan {

		//Input file
		in := inPath + "\\" + fileToMove

		outFolder := createOutputPath(inPath, outPath, fileToMove)

		out := outFolder + "\\" + fileToMove

		//create the directory (by default only if it doesn't exist')
		os.MkdirAll(outFolder, 0666)

		err := os.Rename(in, out)
		if err != nil {
			log.Println(err)
		}
	}

	//Remove go routine from list
	wg.Done()
}

func createOutputPath(inPath string, outPath string, fileToMove string) string {
	fi, err := os.Stat(inPath + "\\" + fileToMove)
	if err != nil {
		log.Println(err)
	}

	var outFolder = ""
	mTime := fi.ModTime()

	year := C.Itoa(mTime.Year())
	month := C.Itoa(int(mTime.Month()))
	day := "Day " + C.Itoa(mTime.Day())

	switch folderType {

	//1 uses file mod time as the folder YYYY\MM\Day DD format
	case "1":
		outFolder = outPath + "\\" + year + "\\" + month + "\\" + day

		return outFolder

		//2 uses the pattern as the folder
	case "2":
		outFolder = outPath + "\\" + pattern

		return outFolder

		//3 uses the pattern as the folder and then groups by year
	case "3":
		outFolder = outPath + "\\" + pattern + "\\" + year
		return outFolder

		//4 will go to the root of defaultOut
	case "4":
		outFolder = outPath + "\\"

		return outFolder

		//5 uses mod time as the folder in YYYYMM format
	case "5":
		month = "0" + month
		month = month[len(month)-2:]
		outFolder = outPath + "\\" + year + month

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

func header() string {
	println("SLOTH: GO Edition")
	println("----------------------")
	return ""
}
