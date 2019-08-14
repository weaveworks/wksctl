package quay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/registry"
	"github.com/weaveworks/wksctl/pkg/utilities/version"
)

// RetagCommands fetches all images in the provided source organization, and
// generates docker pull, tag, and push commands to be able to copy these
// in the provided destination registry and organization.
//
// N.B.: please avoid using this method, it is only kept here as more
// convenient for testing than in ./tools/pharos/retag
func RetagCommands(sourceOrg, destOrg, destRegistry string) ([]string, error) {
	sourceImages, err := ListImages(sourceOrg, version.AnyRange)
	if err != nil {
		return nil, err
	}
	commands := []string{}
	for _, sourceImage := range sourceImages {
		// Make a copy of the source image and overrides the relevant "coordinates":
		destImage := sourceImage
		destImage.Registry = destRegistry
		destImage.User = destOrg
		commands = append(commands, sourceImage.CommandsToRetagAs(destImage)...)
	}
	return commands, nil
}

// imagesBlacklist contains the name of the images to remove from what ListImages returns.
var imagesBlacklist = map[string]struct{}{
	"build":             {}, // Only used internally, no point listing this for end-users.
	"k8s-krb5-server":   {}, // This is only used by the Kerberos addon, and should be listed by our addons' logic instead.
	"mock-authz-server": {}, // Only used by CI, no point listing this for end-users.
	"wks":               {}, // Old Pharos images (wksctl v1).
}

// kubernetesImages contains the name of the Kubernetes images we may want to apply filters on.
var kubernetesImages = map[string]struct{}{
	"kube-apiserver-amd64":          {},
	"kube-controller-manager-amd64": {},
	"kube-proxy-amd64":              {},
	"kube-scheduler-amd64":          {},
}

// ListImages collect all container images in all repositories of the provided
// organization, and returns these as an array of registry.Image objects.
// A Kubernetes versions' range is also required to control which Kubernetes
// images are pulled, are they are numerous, e.g.: >v1.10.0 <=v1.10.12.
func ListImages(organization, k8sVersionsRange string) ([]registry.Image, error) {
	repos, err := getRepos(organization)
	if err != nil {
		return nil, err
	}
	// The retrieval of images, below, is I/O-bound, hence we do it concurrently.
	aborted := make(chan struct{}) // to signal a failure and "fail fast" by canceling goroutines still running
	defer close(aborted)
	results := make(chan result)
	var wg sync.WaitGroup
	for _, r := range repos.Repos {
		if r.Kind == "image" {
			wg.Add(1)
			go func(r repo) {
				defer wg.Done()
				select {
				case results <- listImages(r.Namespace, r.Name, k8sVersionsRange):
				case <-aborted:
					log.WithField("user", r.Namespace).WithField("name", r.Name).Warn("Aborted retrieval of container images.")
				}
			}(r)
		}
	}
	// Synchronise goroutines, to then close the results channel:
	go func() {
		wg.Wait()
		log.Debug("Retrieval of container images: all goroutines terminated.")
		close(results)
		log.Debug("Retrieval of container images: results channel closed.")
	}()
	// Collect all results:
	images := []registry.Image{}
	for result := range results {
		if result.Error != nil {
			// Exiting early here will trigger: defer close(aborted)
			// which in turn will abort all running goroutines.
			return nil, result.Error
		}
		images = append(images, result.Images...)
	}
	sort.Sort(registry.ByCoordinate(images)) // predictable (i.e. sorted) results, despite concurrency.
	return images, nil
}

type result struct {
	Images []registry.Image
	Error  error
}

func listImages(user, name, k8sVersionsRange string) result {
	if _, blacklisted := imagesBlacklist[name]; blacklisted {
		return result{Error: nil, Images: []registry.Image{}}
	}

	image, err := getImage(user, name)
	if err != nil {
		return result{Error: err, Images: nil}
	}

	_, isKubernetesImage := kubernetesImages[name]

	images := make([]registry.Image, 0)
	for tag := range image.Tags {
		if !isKubernetesImage || version.MustMatchRange(semverize(tag), k8sVersionsRange) {
			images = append(images, registry.Image{
				Registry: "quay.io",
				User:     user,
				Name:     image.ImageName,
				Tag:      tag,
			})
		}
	}
	return result{Error: nil, Images: images}
}

func semverize(tag string) string {
	// Remove all "v"s to produce a valid semver, e.g. "v1.2.3" -> "1.2.3"
	// See also: https://github.com/semver/semver/blob/master/semver.md#is-v123-a-semantic-version
	return strings.Replace(tag, "v", "", -1)
}

// repos represents repositories retrievable from quay.io via https://quay.io/api/v1/repository.
type repos struct {
	Repos []repo `json:"repositories"`
}

// repo represents the one repository within a repos object.
type repo struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// image represents a container image available in a repo object.
type image struct {
	Tags      map[string]interface{} `json:"tags"`
	ImageName string                 `json:"name"`
	Namespace string                 `json:"namespace"`
}

func getRepos(namespace string) (*repos, error) {
	reposURL := fmt.Sprintf("https://quay.io/api/v1/repository?popularity=true&public=true&namespace=%s", namespace)
	reposBody, err := httpGet(reposURL)
	if err != nil {
		return nil, err
	}
	defer reposBody.Close()
	repos := &repos{}
	if err := json.NewDecoder(reposBody).Decode(repos); err != nil {
		return nil, err
	}
	return repos, nil
}

func getImage(namespace, name string) (*image, error) {
	imageURL := fmt.Sprintf("https://quay.io/api/v1/repository/%s/%s?includeStats=false", namespace, name)
	imageBody, err := httpGet(imageURL)
	if err != nil {
		return nil, err
	}
	defer imageBody.Close()
	image := &image{}
	if err := json.NewDecoder(imageBody).Decode(image); err != nil {
		return nil, err
	}
	return image, nil
}

func httpGet(url string) (io.ReadCloser, error) {
	var statusCode int
	maxRetries := uint(5)
	for i := uint(0); i < maxRetries; i++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		statusCode = resp.StatusCode
		if statusCode == http.StatusTooManyRequests {
			// Exponentially back-off for 1, 2, 4, 8, 16 seconds:
			time.Sleep(1 << i * time.Second)
			continue
		}
		if statusCode != http.StatusOK {
			return nil, fmt.Errorf("status code was %d", statusCode)
		}
		return resp.Body, nil
	}
	return nil, fmt.Errorf("status code was %d after %d retries", statusCode, maxRetries)
}
