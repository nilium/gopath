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

var gopathFile = ".go-path"
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

func findGopathAboveDir(dir string) (path string, err error) {
	dir, err = filepath.Abs(dir)

outerSearch:
	for err == nil && dir != "/" && dir != "." {
		fpath := filepath.Join(dir, gopathFile)
		fi, err := os.Stat(fpath)
		if (err != nil && !os.IsNotExist(err)) || (err == nil && fi.IsDir()) {
			dir = filepath.Dir(dir)
			continue
		} else if err != nil {
			break outerSearch
		}

		// fpath exists and is a gopath file
		b, err := ioutil.ReadFile(fpath)
		if err != nil {
			err = fmt.Errorf("Error reading %q: %v", fpath, err)
			break outerSearch
		}

		lines := bytes.Split(bytes.Trim(bytes.Replace(b, pathStrip, nil, -1), pathTrim), pathSplit)
		if len(lines) == 0 {
			lines = [][]byte{[]byte(dir)}
		}

		for i, ib := range lines {
			p, err := filepath.Abs(string(ib))
			if err != nil {
				break outerSearch
			}

			lines[i] = []byte(p)
		}

		path = string(bytes.Join(lines, pathJoin))
		break outerSearch
	}

	if path == "" && err == nil {
		err = os.ErrNotExist
	}

	return path, err
}

func main() {
	flag.StringVar(&gopathFile, "marker", gopathFile, "The marker file to indicate a GOPATH entry with. If the file is non-empty, each line is a GOPATH.")
	flag.Parse()

	var args []string
	if flag.NArg() > 0 {
		args = flag.Args()
	} else {
		args = []string{"."}
	}

	paths := make([]string, 0, len(args)+1)
	for _, p := range args {
		p, err := findGopathAboveDir(p)
		switch {
		case os.IsNotExist(err):
			continue
		case err != nil:
			log.Panic("ERROR:", err)
		default:
		}
		paths = append(paths, p)
	}

	if gopath := os.Getenv("GOPATH"); len(gopath) > 0 {
		paths = append(paths, gopath)
	}

	io.WriteString(os.Stdout, strings.Join(paths, ":"))
	if isTTY() {
		io.WriteString(os.Stdout, "\n")
	}
}
