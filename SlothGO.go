package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
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
var Pattern string
var folderType string

type Folder struct {
	Name       string `json:"name"`
	Input      string `json:"input"`
	Output     string `json:"output"`
	Pattern    string `json:"pattern"`
	FolderType string `json:"folderType"`
}

func main() {
	println("SLOTH: GO Edition")
	println("----------------------")
	start := time.Now()
	log.Println("Start time:", start)

	folders := getFolders()

	for _, f := range folders {
		name = f.Name
		inPath = f.Input
		outPath = f.Output
		Pattern = f.Pattern
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
			go MoveFiles(readChan)
		}

		//Iterate over each file and move it
		for _, element := range files {
			if !element.IsDir() {
				if string([]byte(element.Name())[len(element.Name())-len(Pattern):]) == Pattern {
					//Count number of go routines
					readChan <- element.Name()
					//println(element.Name())
				}
			}
		}

		close(readChan)

		//Wait for all go routines to finish
		wg.Wait()

		//notify(inPath, outPath, Pattern)

		elapsed := time.Since(start)
		fmt.Printf("Execution Time: %.2f seconds to run %s\n", elapsed.Seconds(), name)
	}
}

func MoveFiles(inChan chan string) {

	for fileToMove := range inChan {

		//Input file
		in := inPath + "\\" + fileToMove

		outFolder := CreateOutputPath(inPath, outPath, fileToMove)

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

func CreateOutputPath(inPath string, outPath string, fileToMove string) string {
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
		outFolder = outPath + "\\" + Pattern

		return outFolder

	//3 uses the pattern as the folder and then groups by year
	case "3":
		outFolder = outPath + "\\" + Pattern + "\\" + year
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

func getFolders() []Folder {
	raw, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println("getFolders -", err.Error())
		os.Exit(1)
	}

	var c []Folder
	json.Unmarshal(raw, &c)
	return c
}

func notify(inPath, outPath, Pattern string) {
	values := map[string]string{"value1": inPath, "value2": outPath, "value3": Pattern}
	jsonValue, _ := json.Marshal(values)

	//Notify IFTTT.com custom Maker channel
	resp, err := http.Post("https://maker.ifttt.com/trigger/Sloth_Notify/with/key/cRoTTDKR6fNC2X1MifxRyW", "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		log.Println(resp.Status)
	}

	println("Notification Sent to IFTTT.com", resp.Status)
}
