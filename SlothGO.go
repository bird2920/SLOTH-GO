package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	C "strconv"
	"sync"
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
	fType := flag.String("folderType", "3", "1 \\moddate, 2 \\pattern, 3 none")

	flag.Parse()

	inPath = *in
	outPath = *out
	Pattern = *pattern
	folderType = *fType

	fmt.Println("Input Path " + inPath)
	fmt.Println("Output Path " + outPath)
	fmt.Println("Pattern " + Pattern)
	fmt.Println("Folder Type " + folderType)

	readChan = make(chan string, 250)

	//Is the file a file?
	files, err := ioutil.ReadDir(inPath)
	if err != nil {
		println(err.Error())
	}

	//Start workers
	for i := 0; i < 250; i++ {
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
}

func MoveFiles(inChan chan string) {

	for fileToMove := range inChan {
		//Input file
		in := inPath + "\\" + fileToMove

		fmt.Println(in)

		outFolder := CreateOutputPath(inPath, outPath, fileToMove)
		out := outFolder + "\\" + fileToMove

		//create the directory (by default only if it doesn't exist')
		os.MkdirAll(outFolder, os.ModePerm)

		err := os.Rename(in, out)
		if err != nil {
			log.Fatal(err)
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

	switch folderType {

	//1 uses file mod time as the folder
	case "1":
		mTime := fi.ModTime()

		year := C.Itoa(mTime.Year())
		month := mTime.Month().String() //The month field is coming across as the month name.
		day := "Day " + C.Itoa(mTime.Day())

		outFolder = outPath + "\\" + year + "\\" + month + "\\" + day

		return outFolder

	//2 uses the pattern as the folder
	case "2":
		outFolder = outPath + "\\" + Pattern

		return outFolder

	//3 will go to the root of defaultOut
	case "3":
		outFolder = outPath + "\\"

		return outFolder

	default:
		return ""
	}
}
