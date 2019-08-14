package addons

import (
	"errors"
	"fmt"
	"strings"
)

// imageReference represents the coordinates of a container image, typically of
// the following shape:
// ${DOMAIN[:PORT]}/${ORGANISATION}/${NAME}[:${TAG}][@sha256:${DIGEST}]
type imageReference struct {
	Domain       string
	Organisation string
	Name         string
	Tag          string
	Digest       string
}

const (
	// NameTotalLengthMax is the maximum total number of characters in a repository name.
	NameTotalLengthMax = 255
)

var (
	// ErrReferenceInvalidFormat represents an error while trying to parse a string as a reference.
	ErrReferenceInvalidFormat = errors.New("invalid reference format")

	// ErrTagInvalidFormat represents an error while trying to parse a string as a tag.
	ErrTagInvalidFormat = errors.New("invalid tag format")

	// ErrDigestInvalidFormat represents an error while trying to parse a string as a tag.
	ErrDigestInvalidFormat = errors.New("invalid digest format")

	// ErrNameContainsUppercase is returned for invalid repository names that contain uppercase characters.
	ErrNameContainsUppercase = errors.New("repository name must be lowercase")

	// ErrNameEmpty is returned for empty, invalid repository names.
	ErrNameEmpty = errors.New("repository name must have at least one component")

	// ErrNameTooLong is returned when a repository name is longer than NameTotalLengthMax.
	ErrNameTooLong = fmt.Errorf("repository name must not be more than %v characters", NameTotalLengthMax)

	// ErrNameNotCanonical is returned when a name is not canonical.
	ErrNameNotCanonical = errors.New("repository name must be canonical")
)

func parseImageReference(s string) (*imageReference, error) {
	matches := ReferenceRegexp.FindStringSubmatch(s)
	if matches == nil {
		if s == "" {
			return nil, ErrNameEmpty
		}
		if ReferenceRegexp.FindStringSubmatch(strings.ToLower(s)) != nil {
			return nil, ErrNameContainsUppercase
		}
		return nil, ErrReferenceInvalidFormat
	}

	if len(matches[1]) > NameTotalLengthMax {
		return nil, ErrNameTooLong
	}

	ref := imageReference{
		Domain:       matches[1],
		Organisation: matches[2],
		Name:         matches[3],
		Tag:          matches[4],
		Digest:       matches[5],
	}

	// We might have greedily and incorrectly captured the organisation as the
	// domain, hence shift the path components to the right:
	if ref.Organisation == "" &&
		// A dot is more likely to be present in a domain name, than an organisation?
		!strings.Contains(ref.Domain, ".") &&
		// A semi-colon is more likely to be present an endpoint, than an organisation?
		!strings.Contains(ref.Domain, ":") {
		ref.Organisation = ref.Domain
		ref.Domain = ""
	}

	return &ref, nil
}

func (r *imageReference) String() string {
	var s strings.Builder
	if r.Domain != "" {
		s.WriteString(r.Domain)
		s.WriteString("/")
	}
	if r.Organisation != "" {
		s.WriteString(r.Organisation)
		s.WriteString("/")
	}
	s.WriteString(r.Name)
	if r.Tag != "" {
		s.WriteString(":")
		s.WriteString(r.Tag)
	}
	if r.Digest != "" {
		s.WriteString("@")
		s.WriteString(r.Digest)
	}
	return s.String()
}
