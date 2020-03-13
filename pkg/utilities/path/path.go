package path

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
)

// UserHomeDirectory returns the user directory.
func UserHomeDirectory() (string, error) {
	currentUser, err := user.Current()
	if err == nil {
		return currentUser.HomeDir, nil
	}

	home := os.Getenv("HOME")
	if home != "" {
		return home, nil
	}

	return "", errors.New("failed to find user home directly")
}

// Expand expands the provided path, evaluating all symlinks (including "~").
func Expand(path string) (string, error) {
	path = expandHome(path)
	return filepath.EvalSymlinks(path)
}

func expandHome(s string) string {
	home, _ := UserHomeDirectory()
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(home, s[2:])
	}
	return s
}

// WKSHome sanitises the provided (optional) artifact directory or defaults it.
func WKSHome(artifactDirectory string) string {
	// Command line option overrides the default home directory.
	if artifactDirectory != "" {
		return expandHome(artifactDirectory)
	}
	return clientcmd.RecommendedHomeFile
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
