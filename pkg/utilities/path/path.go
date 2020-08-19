package path

import (
	"fmt"
	"os"
	"path/filepath"

// Expand expands the provided path, evaluating all symlinks (including "~").
func Expand(path string) (string, error) {
	path = ExpandHome(path)
	return filepath.EvalSymlinks(path)
}

func ExpandHome(s string) string {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(home, s[2:])
	}
	return s
}

// WKSHome sanitises the provided (optional) artifact directory or defaults it.
func WKSHome(artifactDirectory string) string {
	// Command line option overrides the default home directory.
	if artifactDirectory != "" {
		return ExpandHome(artifactDirectory)
	}
	if userHome, err := os.UserHomeDir(); err == nil {
		return filepath.Join(userHome, ".wks")
	}
	wd, _ := os.Getwd()
	return wd
}

// WKSResourcePath joins the provided (optional) artifact directory and the
// provided path components into a well-formed path.
func WKSResourcePath(artifactDirectory string, paths ...string) string {
	args := []string{WKSHome(artifactDirectory)}
	args = append(args, paths...)
	return filepath.Join(args...)
}

// CreateDirectory creates directories corresponding to the provided path.
func CreateDirectory(path string) (string, error) {
	// Create wksHome if it doesn't exist, or ensure it's a directory if it does
	if wksHomeStat, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("Error stating: %v", err)
		}
		if err := os.MkdirAll(path, 0755); err != nil {
			return "", fmt.Errorf("Error creating: %v", err)
		}
	} else {
		if !wksHomeStat.IsDir() {
			return "", fmt.Errorf("Not a directory: %v", path)
		}
	}
	return path, nil
}

func Kubeconfig(artifactDirectory, ns, clusterName string) string {
	return filepath.Join(WKSResourcePath(artifactDirectory, ns, clusterName), "kubeconfig")
}
