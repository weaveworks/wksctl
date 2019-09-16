package utilities

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

//CreateFile creates the file named in fname or a file named defaultName in the directory named in fname
func CreateFile(fname, defaultName string) (*os.File, error) {
	if isDir(fname) {
		// Create the default completion file in the directory
		f, err := CreateFile(filepath.Join(fname, defaultName), "")
		if err != nil {
			log.Fatal(err)
		}
		return f, nil
	}
	f, err := os.Create(fname)
	if err != nil {
		log.Fatal(err)
	}
	return f, nil

}

/*
 Heuristically determines if the passed in filename is a directory

 A filename is determined to be a directory if
 - the filename ends in "/"
 - the filename matches an existing directory
*/
func isDir(fname string) bool {
	if strings.HasSuffix(fname, "/") {
		// create the directory if it doesn't exist
		if err := os.MkdirAll(fname, 0755); err != nil {
			log.Fatal(err)
		}
		return true
	}
	finfo, err := os.Stat(fname)
	if err == nil {
		return finfo.IsDir()
	}
	return false
}
