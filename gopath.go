package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var pathSplit = []byte{byte('\n')}
var pathJoin = []byte{byte(':')}
var pathTrim = "\n"
var pathStrip = []byte{byte('\r')}

// isTTY attempts to determine whether the current stdout refers to a terminal.
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error getting Stat of os.Stdout:", err)
		return true // Assume human readable
	}
	return (fi.Mode() & os.ModeNamedPipe) != os.ModeNamedPipe
}

func joinGopathFile(dir, path, gopath string, includeDir bool) string {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("Error reading %q: %v", path, err)
		return gopath
	}

	lines := bytes.Split(bytes.Trim(bytes.Replace(b, pathStrip, nil, -1), pathTrim), pathSplit)
	if len(lines) == 0 || (len(lines) == 1 && len(lines[0]) == 0) {
		if includeDir {
			lines = [][]byte{[]byte(dir)}
		} else {
			return gopath
		}
	}

	for i, ib := range lines {
		newpath := string(ib)
		if len(newpath) == 0 {
			newpath = dir
		} else if !filepath.IsAbs(newpath) {
			newpath = filepath.Join(dir, newpath)
		}
		p, err := filepath.Abs(newpath)
		if err != nil {
			// Skip path if an error occurred making it absolute
			continue
		}

		lines[i] = []byte(p)
	}

	found := string(bytes.Join(lines, pathJoin))
	if len(gopath) > 0 {
		gopath = gopath + ":" + found
	} else {
		gopath = found
	}
	return gopath
}

// findGopathAboveDir searches for a markerFile representing one or more GOPATH
// entries in the directory given and all directories above it. If toRoot is
// false, it will stop at the first markerFile found.
func findGopathAboveDir(dir, markerFile string, toRoot bool) (path string, err error) {
	dir, err = filepath.Abs(dir)

outerSearch:
	for err == nil {
		fpath := filepath.Join(dir, markerFile)
		fi, err := os.Stat(fpath)
		if !(os.IsNotExist(err) || (err == nil && fi.IsDir())) {
			path = joinGopathFile(dir, fpath, path, true)
		}

		fpath = filepath.Join(dir, ".gocfg", "gopaths")
		fi, err = os.Stat(fpath)
		if !(os.IsNotExist(err) || (err == nil && fi.IsDir())) {
			path = joinGopathFile(dir, fpath, path, false)
		}

		if !toRoot || dir == "/" || dir == "." {
			err = nil
			break outerSearch
		}
		dir = filepath.Dir(dir)
	}

	if path == "" && err == nil {
		err = os.ErrNotExist
	} else if err != nil {
		path = ""
	}

	return path, err
}

func main() {
	// CLI options
	var (
		gopathFile   string = ".go-path"
		searchToRoot bool   = false
	)

	flag.StringVar(&gopathFile, "marker", gopathFile, "The marker file to indicate a GOPATH entry with. If the file is non-empty, each line is a GOPATH.")
	flag.BoolVar(&searchToRoot, "to-root", searchToRoot, "Whether to continue searching up to the root even after a GOPATH entry is found.")

	flag.Parse()

	// If no arguments, use CWD.
	var args []string
	if flag.NArg() > 0 {
		args = flag.Args()
	} else {
		wd, err := os.Getwd()
		if err != nil {
			log.Fatal("Error getting working directory:", err)
		}
		args = []string{wd}
	}

	// Enumerate paths, generating GOPATHs for each one
	paths := make([]string, 0, len(args)+1)
	for _, p := range args {
		p, err := findGopathAboveDir(p, gopathFile, searchToRoot)
		switch {
		case os.IsNotExist(err):
			continue
		case err != nil:
			log.Fatal("ERROR:", err)
		default:
		}
		paths = append(paths, p)
	}

	// Then join each GOPATH string
	if gopath := os.Getenv("GOPATH"); len(gopath) > 0 {
		paths = append(paths, gopath)
	}

	// Remove duplicate entries, retain order
	result := strings.Split(strings.Join(paths, ":"), ":")
	found := make(map[string]bool, len(result))
	unique := make([]string, 0, len(result))
	for _, p := range result {
		if found[p] {
			continue
		}
		found[p] = true
		unique = append(unique, p)
	}

	// Join paths into final GOPATH
	GOPATH := strings.Join(unique, ":")

	io.WriteString(os.Stdout, GOPATH)
	if isTTY() {
		io.WriteString(os.Stdout, "\n")
	}
}
