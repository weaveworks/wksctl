package test

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func downloadFile(filepath, url string) error {
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func downloadFileWithRetries(filepath, url string, retries int) error {
	var err error
	for i := 0; i < retries; i++ {
		err = downloadFile(filepath, url)
		if err == nil {
			return nil
		}
	}

	return err
}
