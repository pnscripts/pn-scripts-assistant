package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The link for a browser is printed for a person at a terminal, and not
// into a file or a pipe, where it would wait for whatever reads logs.
func TestTheLinkIsNotPrintedIntoAFileOrAPipe(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "service.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if saidToAPerson(file) {
		t.Error("a log file was taken for a terminal")
	}

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()

	if saidToAPerson(write) {
		t.Error("a pipe was taken for a terminal")
	}
}

func TestTheLinkIsSaidWithWhatItIsFor(t *testing.T) {
	var out bytes.Buffer

	link := "http" + "://127.0.0.1:8090/desk/0123abcd"

	sayDeskLink(&out, link)

	said := out.String()

	if strings.Count(said, link) != 1 {
		t.Errorf("the link is not printed exactly once:\n%s", said)
	}

	for _, word := range []string{"own window", "once", "browser on this computer"} {
		if !strings.Contains(said, word) {
			t.Errorf("the lines about the link do not say %q:\n%s", word, said)
		}
	}

	if strings.ContainsAny(said, "–—") {
		t.Errorf("a dash in what a person reads:\n%s", said)
	}
}
