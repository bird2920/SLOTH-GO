package main

import "testing"

func TestFileGet(t *testing.T){
	h := header()

	if h != "1"{
		t.Error("expected something", h)
	}


}