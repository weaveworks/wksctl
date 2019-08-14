package assets

import (
	"io/ioutil"
)

// ReadAll reads all content from path.
func ReadAll(path string) (string, error) {
	f, err := Assets.Open(path)
	if err != nil {
		return "", err
	}
	d, err := ioutil.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(d), nil
}
