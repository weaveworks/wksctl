package addons

import (
	"fmt"
	"net/http"
	"os"
	"path"

	"github.com/google/go-jsonnet"
	"github.com/weaveworks/wksctl/pkg/addons/assets"
)

// vfsImport implements a jsonnet VM Importer for vfsgen static data.
type vfsImporter struct {
	searchPaths []string
	assets      http.FileSystem
	cache       map[string]*cacheEntry
}

type cacheEntry struct {
	exists   bool
	contents jsonnet.Contents
}

func newVFSImporter() *vfsImporter {
	return &vfsImporter{
		cache: make(map[string]*cacheEntry),
	}
}

func (importer *vfsImporter) tryPath(dir, importedPath string) (found bool, contents jsonnet.Contents, foundHere string, err error) {
	var absPath string
	if path.IsAbs(importedPath) {
		absPath = importedPath
	} else {
		absPath = path.Join(dir, importedPath)
	}

	entry := importer.cache[absPath]
	if entry == nil {
		// Build cache entry.
		s, err := assets.ReadAll(absPath)
		if os.IsNotExist(err) {
			entry = &cacheEntry{
				exists: false,
			}
		} else {
			entry = &cacheEntry{
				exists:   true,
				contents: jsonnet.MakeContents(s),
			}
		}

		importer.cache[absPath] = entry
	}

	return entry.exists, entry.contents, absPath, nil
}

func (importer *vfsImporter) Import(importedFrom, importedPath string) (contents jsonnet.Contents, foundAt string, err error) {
	dir, _ := path.Split(importedFrom)
	found, content, foundHere, err := importer.tryPath(dir, importedPath)

	for i := len(importer.searchPaths) - 1; !found && i >= 0; i-- {
		found, content, foundHere, err = importer.tryPath(importer.searchPaths[i], importedPath)
		if err != nil {
			return jsonnet.Contents{}, "", err
		}
	}

	if !found {
		return jsonnet.Contents{}, "", fmt.Errorf("couldn't open import %#v: no match locally or in the Jsonnet library paths", importedPath)
	}
	return content, foundHere, err

}
