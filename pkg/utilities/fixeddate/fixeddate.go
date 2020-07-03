// Package fixeddate implements a http.FileSystem that gives each file a fixed date.
// This is used with the vfsdata generator to avoid spurious diffs.
package fixeddate

import (
	"net/http"
	"os"
	"time"
)

// Dir is a wrapper around http.Dir that gives every file a fixed date.
type Dir string

// Open implements http.FileSystem
func (f Dir) Open(name string) (http.File, error) {
	dir, err := http.Dir(f).Open(name)
	return fixedDateFile{File: dir}, err
}

type fixedDateFile struct {
	http.File
}

// Stat overrides the same method in http.File
func (f fixedDateFile) Stat() (os.FileInfo, error) {
	info, err := f.File.Stat()
	return fixedDateFileInfo{FileInfo: info}, err
}

type fixedDateFileInfo struct {
	os.FileInfo
}

var fixedDate, _ = time.Parse(time.RFC3339, "2020-01-01T00:00:00Z")

// ModTime overrides the same method in os.FileInfo
func (f fixedDateFileInfo) ModTime() time.Time {
	return fixedDate
}
