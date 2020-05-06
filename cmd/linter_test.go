package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"
)

func TestLinter(t *testing.T) {
	*write = true

	matches, err := filepath.Glob("testdata/*.input")
	if err != nil {
		t.Fatal(err)
	}

	for _, in := range matches {
		gld := in[:len(in)-len(".input")] + ".golden"
		t.Run(in, func(t *testing.T) {
			runTest(t, in, gld)
			if in != gld {
				// Check idempotence
				runTest(t, gld, gld)
			}
		})
	}
}

func runTest(t *testing.T, in, gld string) {
	// Copy the input file to a temporary directory
	dir, err := ioutil.TempDir("", "")
	assertOk(t, err)
	defer os.RemoveAll(dir)

	inBytes, err := ioutil.ReadFile(in)
	assertOk(t, err)
	file := path.Join(dir, "main.go")
	err = ioutil.WriteFile(file, inBytes, os.ModePerm)
	assertOk(t, err)

	// Apply the linter to the directory
	err = walkDir(dir)
	assertOk(t, err)

	// Compare the linted file with the golden file
	outBytes, err := ioutil.ReadFile(file)
	assertOk(t, err)
	gldBytes, err := ioutil.ReadFile(gld)
	assertOk(t, err)
	if !bytes.Equal(outBytes, gldBytes) {
		t.Errorf(
			"Output file doesn't match golden file.\n-- Input:\n%s\n-- Output:\n%s\n-- Golden:\n%s\n",
			string(inBytes),
			string(outBytes),
			string(gldBytes))
	}
}

func assertOk(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
