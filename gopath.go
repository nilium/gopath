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

func logf(f string, args ...interface{}) {
	if loud {
		log.Printf(f, args...)
	}
}

var pathSplit = []byte{byte('\n')}
var pathJoin = []byte{byte(':')}
var pathTrim = "\n"
var pathStrip = []byte{byte('\r')}
var loud bool = false

// isTTY attempts to determine whether the current stdout refers to a terminal.
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error getting Stat of os.Stdout:", err)
		return true // Assume human readable
	}
	return (fi.Mode() & os.ModeNamedPipe) != os.ModeNamedPipe
}

var prefixpad = strings.Repeat(" ", 30)

func logprefix(p string) {
	p = trunc(p)
	if len(p) < len(prefixpad) {
		p = prefixpad[:len(prefixpad)-len(p)] + p
	}
	log.SetPrefix(p)
}

func trunc(p string) string {
	if len(p) > 30 {
		return "…" + p[len(p)-29:]
	}
	return p
}

func joinGopathFile(dir, path, gopath string, includeDir bool) (next string, stop, drop bool) {
	defer log.SetPrefix(log.Prefix())
	logprefix(path + ": ")

	b, err := ioutil.ReadFile(path)
	if err != nil {
		logf("Error reading %q: %v", path, err)
		return gopath, false, false
	}

	lines := bytes.Split(bytes.Trim(bytes.Replace(b, pathStrip, nil, -1), pathTrim), pathSplit)
	if len(lines) == 0 || (len(lines) == 1 && len(lines[0]) == 0) {
		if includeDir {
			lines = [][]byte{[]byte(dir)}
		} else {
			return gopath, false, false
		}
	}

	keep := make([][]byte, 0, len(lines))
	for _, ib := range lines {
		newpath := string(ib)
		if len(newpath) == 0 || newpath[0] == '#' {
			continue
		} else if newpath == "-" {
			logf("Dropping environment path entries")
			drop = true
			continue
		} else if newpath == "!" {
			logf("Stopping directory search")
			stop = true
			continue
		} else if !filepath.IsAbs(newpath) {
			newpath = filepath.Join(dir, newpath)
		}
		p, err := filepath.Abs(newpath)
		if err != nil {
			logf("Error getting path for %q: %v", newpath, err)
			// Skip path if an error occurred making it absolute
			continue
		}

		logf("Keeping %s", p)
		keep = append(keep, []byte(p))
	}

	found := string(bytes.Join(keep, pathJoin))
	if len(gopath) > 0 {
		gopath = gopath + ":" + found
	} else {
		gopath = found
	}
	logf("%s (stop=%t drop=%t)", gopath, stop, drop)
	return gopath, stop, drop
}

// findGopathAboveDir searches for a markerFile representing one or more GOPATH
// entries in the directory given and all directories above it. If toRoot is
// false, it will stop at the first markerFile found.
func findGopathAboveDir(dir, markerFile string, toRoot bool) (stop, drop bool, path string, err error) {
	defer log.SetPrefix(log.Prefix())

	dir, err = filepath.Abs(dir)
	var paths []string

outerSearch:
	for err == nil && !stop {
		logprefix(dir + ": ")
		var fpath string
		fpath = filepath.Join(dir, markerFile)
		if markerFile != "" {
			logf("Looking for marker: %s", fpath)
			fi, err := os.Stat(fpath)
			if !(os.IsNotExist(err) || (err == nil && fi.IsDir())) {
				logf("Reading marker: %s", fpath)
				pathset, stopAfter, dropAfter := joinGopathFile(dir, fpath, "", true)
				stop = stop || stopAfter
				drop = drop || dropAfter

				if len(pathset) > 0 {
					logf("Appending marker pathset: %v", pathset)
					paths = append(paths, pathset)
				}
			}
		}

		// wgo support
		logf("Looking for wgo gopaths: %s", fpath)
		fpath = filepath.Join(dir, ".gocfg", "gopaths")
		fi, err := os.Stat(fpath)
		if !(os.IsNotExist(err) || (err == nil && fi.IsDir())) {
			logf("Reading wgo gopaths: %s", fpath)
			pathset, stopAfter, dropAfter := joinGopathFile(dir, fpath, "", false)
			stop = stop || stopAfter
			drop = drop || dropAfter

			if len(pathset) > 0 {
				logf("Appending wgo pathset: %v", pathset)
				paths = append(paths, pathset)
			}
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
		paths = nil
	}

	return stop, drop, strings.Join(paths, ":"), err
}

func main() {
	log.SetFlags(0)
	logprefix("(gopath): ")

	// CLI options
	var (
		gopathFile   string = ".go-path"
		searchToRoot bool   = true
		envKey       string = "GOPATH"
	)

	flag.StringVar(&envKey, "env", envKey, "The environment variable to read existing path entries from.")
	flag.StringVar(&gopathFile, "marker", gopathFile, "The marker `filename` to read path entries from. If the file is non-empty, each line is a path entry, otherwise the directory of the file is a path entry.")
	flag.BoolVar(&searchToRoot, "to-root", searchToRoot, "Whether to continue searching up to the root directory after a marker is found.")
	flag.BoolVar(&loud, "verbose", loud, "Whether to emit log messages in case of an error. If false, no log messages are printed.")

	flag.Parse()

	// Grab current GOPATH
	var GOPATH string
	if envKey != "" {
		GOPATH = os.Getenv(envKey)
	}

	// If no arguments, use CWD.
	var args []string
	if flag.NArg() > 0 {
		args = flag.Args()
	} else if wd, err := os.Getwd(); err != nil {
		logf("Error getting working directory:", err)
		goto end
	} else {
		args = []string{wd}
	}

	{
		// Enumerate paths, generating GOPATHs for each one
		paths := make([]string, 0, len(args)+1)
		keepEnv := true

	pathloop:
		for _, p := range args {
			stop, drop, p, err := findGopathAboveDir(p, gopathFile, searchToRoot)
			logf("p=%v", p)

			if drop {
				keepEnv = false
			}

			switch {
			case os.IsNotExist(err):
				logf("Skipping not-found error: %v", err)
			case err != nil:
				logf("ERROR: %v", err)
				break pathloop
			default:
			}

			if len(p) > 0 {
				logf("Appending pathset %v", p)
				paths = append(paths, p)
			}

			if stop {
				logf("Ending search")
				break pathloop
			}
		}

		if keepEnv {
			logf("Appending environment pathset %v", GOPATH)
			paths = append(paths, GOPATH)
		}

		logf("Filtering unique paths: %v", paths)

		// Remove duplicate entries, retain order
		result := strings.Split(strings.Join(paths, ":"), ":")
		found := make(map[string]struct{}, len(result))
		unique := make([]string, 0, len(result))
		for i, p := range result {
			if _, ok := found[p]; ok {
				logf("Dropping duplicate entry %d: %s", i, p)
				continue
			}
			found[p] = struct{}{}
			unique = append(unique, p)
		}

		// Join paths into final GOPATH
		GOPATH = strings.Join(unique, ":")
	}

end:
	io.WriteString(os.Stdout, GOPATH)
	if isTTY() {
		io.WriteString(os.Stdout, "\n")
	}
}
