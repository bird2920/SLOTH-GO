package main

import (
	"log"
	"os"
	"path/filepath"
	C "strconv"
	"testing"
)

func TestGetFolders(t *testing.T) {
	got, _ := getFolders()

	if got == nil {
		t.Errorf("empty return: got %s, want folder path", got)
	}
}

func BenchmarkGetFolders(b *testing.B) {
	getFolders()
}

func Test_createOutputPath(t *testing.T) {
	inPath := "/Users/richardbi/go/src/github.com/bird2920/sloth-go/TestFiles"
	outPath := "/Users/richardbi/go/src/github.com/bird2920/sloth-go/TestFiles"
	fileToMove := "TestFile.txt"
	extension = "txt"

	fi, err := os.Stat(filepath.Join(inPath, fileToMove))
	if err != nil {
		log.Println(err)
	}

	mTime := fi.ModTime()

	year := C.Itoa(mTime.Year())
	month := C.Itoa(int(mTime.Month()))
	day := "Day " + C.Itoa(mTime.Day())

	ti := mTime

	type args struct {
		inPath     string
		outPath    string
		fileToMove string
		folderType string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.

		{"Test foldertype 1", args{inPath, outPath, fileToMove, "1"}, filepath.Join(outPath, year, month, day)},
		{"Test foldertype 2", args{inPath, outPath, fileToMove, "2"}, filepath.Join(outPath, extension)},
		{"Test foldertype 3", args{inPath, outPath, fileToMove, "3"}, filepath.Join(outPath, extension, year)},
		{"Test foldertype 4", args{inPath, outPath, fileToMove, "4"}, outPath},
		{"Test foldertype 5", args{inPath, outPath, fileToMove, "5"}, filepath.Join(outPath, ti.Format("200601"))},
		{"Test foldertype default", args{inPath, outPath, fileToMove, ""}, ""},
		{"Test no filetomove", args{inPath, outPath, "", ""}, ""},
		{"Test no outpath", args{inPath, "", fileToMove, ""}, ""},
		//{"Test no inpath", args{"", outPath, fileToMove, ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folderType = tt.args.folderType
			if got := createOutputPath(tt.args.inPath, tt.args.outPath, tt.args.fileToMove); got != tt.want {
				t.Errorf("createOutputPath() = %v, want %v", got, tt.want)
			}
		})
	}
}
