package main

import (
	"testing"
)

func TestGetFolders(t *testing.T) {
	got := getFolders()

	if got == nil {
		t.Errorf("empty return: got %s, want folder path", got)
	}
}

func Test_moveFiles(t *testing.T) {
	type args struct {
		inChan chan string
	}
	tests := []struct {
		name string
		args args //<----
	}{
		// TODO: Add test cases.
		//this is what I was takling about on the phone the other day

	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moveFiles(tt.args.inChan)
		})
	}
}

func Test_createOutputPath(t *testing.T) {
	type args struct {
		inPath     string
		outPath    string
		fileToMove string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := createOutputPath(tt.args.inPath, tt.args.outPath, tt.args.fileToMove); got != tt.want {
				t.Errorf("createOutputPath() = %v, want %v", got, tt.want)
			}
		})
	}
}
