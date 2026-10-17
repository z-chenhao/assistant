package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSearchStatus(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--status", "paused"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "[{\"Title\":\"Review\",\"Status\":\"paused\"}]\n" {
		t.Fatal(out.String())
	}
}
func TestSearchDefault(t *testing.T) {
	var out bytes.Buffer
	if err := run(nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Build") || !strings.Contains(out.String(), "Review") {
		t.Fatal(out.String())
	}
}
