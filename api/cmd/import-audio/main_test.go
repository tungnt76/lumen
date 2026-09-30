package main

import (
	"reflect"
	"testing"

	"github.com/tony/lumen/api/internal/store"
)

func TestStalePrefixes(t *testing.T) {
	old := []store.Track{
		{Audio: "audio/3/100/001.m4a"},
		{Audio: "audio/3/100/002.m4a"},
		{Audio: "audio/3/200/001.m4a"},
		{Audio: "https://archive.org/download/x/1.mp3"},
		{Audio: "hls/3/master.m3u8"}, // never touch anything outside audio/
	}
	got := stalePrefixes(old, "audio/3/200/")
	if want := []string{"audio/3/100/"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := stalePrefixes(old, ""); len(got) != 2 {
		t.Fatalf("delete: got %v", got)
	}
}
