package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	C "strconv"
	"strings"
	"sync"
	"time"
	"net/http"
	"encoding/json"
	"bytes"
)

var wg sync.WaitGroup
var readChan chan string
var inPath string
var outPath string
var Pattern string
var folderType string

func main() {
	println("SLOTH: GO Edition")
	println("----------------------")
	start := time.Now()
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	defaultIn := usr.HomeDir + "\\Downloads"
	defaultOut := usr.HomeDir + "\\Downloads\\Sloth"

	//InputPath
	in := flag.String("inPath", defaultIn, "readPath")
	if in == nil {

	}

	//OutputPath
	out := flag.String("outPath", defaultOut, "readPath")

	//Pattern
	pattern := flag.String("pattern", "zip", "file search pattern")

	//Folder Type
	fType := flag.String("folderType", "4", "1 \\moddate, 2 \\pattern, 3 \\pattern\\moddate year, 4 none")

	flag.Parse()

	inPath = *in
	outPath = *out
	Pattern = *pattern
	folderType = *fType

	fmt.Println("Input Path: " + inPath)
	fmt.Println("Output Path: " + outPath)
	fmt.Println("Pattern: " + strings.ToUpper(Pattern))
	fmt.Println("Folder Type: " + folderType)

	readChan = make(chan string, 1000)

	//Is the file a file?
	files, err := ioutil.ReadDir(inPath)
	if err != nil {
		println(err.Error())
	}

	//Start workers
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go MoveFiles(readChan)
	}

	//Iterate over each file and move it
	for _, element := range files {
		if !element.IsDir() {
			if string([]byte(element.Name())[len(element.Name())-len(Pattern):]) == Pattern {
				//Count number of go routines
				readChan <- element.Name()
			}
		}
	}

	close(readChan)

	//Wait for all go routines to finish
	wg.Wait()

	values := map[string]string{"value1": inPath, "value2" : outPath, "value3": Pattern}
	jsonValue, _ := json.Marshal(values)

	//Notify IFTTT.com custom Maker channel
	resp, err := http.Post("https://maker.ifttt.com/trigger/Sloth_Notify/with/key/cRoTTDKR6fNC2X1MifxRyW", "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		println(resp.Status)
	}

	println("Notification Sent to IFTTT.com", resp.Status)

	elapsed := time.Since(start)
	println("Execution Time: ", elapsed.Minutes())
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
			log.Fatal(err)
			println(err)
		}
	}

	//Remove go routine from list
	wg.Done()
}

func CreateOutputPath(inPath string, outPath string, fileToMove string) string {
	fi, err := os.Stat(inPath + "\\" + fileToMove)
	if err != nil {
		log.Fatal(err)
	}

	var outFolder = ""
	mTime := fi.ModTime()

	year := C.Itoa(mTime.Year())
	month := C.Itoa(int(time.Now().Month()))
	day := "Day " + C.Itoa(mTime.Day())

	switch folderType {

	//1 uses file mod time as the folder
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

	default:
		return ""
	}
}
