// Package stack holds the stack configuration of the metrik Pulumi project: where each release
// takes its image from and how the cluster gets registry pull credentials.
package stack

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// CreatedPullSecretName is the name of the pull secret created in each release namespace when
// the stack holds registry credentials.
const CreatedPullSecretName = "registry-credentials"

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Release is one entry of metrik:releases.
type Release struct {
	Name                string
	Namespace           string
	ChartPath           string
	ChartValuesFilePath string
	ChartVersion        string
	// Image overrides the image of the values file when set.
	Image *Image
}

// Image is a registry image reference, optionally pinned to a digest.
type Image struct {
	Repository string
	Tag        string
	Digest     string
}

// Registry holds the stack's pull-credential settings. The password itself stays a Pulumi
// secret; only whether it is set matters here.
type Registry struct {
	Server          string
	Username        string
	PasswordSet     bool
	ImagePullSecret string
}

// Mode is how the cluster gets registry pull credentials.
type Mode int

const (
	// ModeNone adds no pull secret (mode c): nodes pull with their own identity.
	ModeNone Mode = iota
	// ModeCreated creates a pull secret from the stack credentials (mode a).
	ModeCreated
	// ModeExisting references a pull secret that already exists (mode b).
	ModeExisting
)

// Mode returns the pull-credential mode of a valid registry configuration.
func (r Registry) Mode() Mode {
	switch {
	case r.Server != "" && r.Username != "" && r.PasswordSet:
		return ModeCreated
	case r.ImagePullSecret != "":
		return ModeExisting
	default:
		return ModeNone
	}
}

// Validate reports every invalid image or registry setting, so a bad stack fails at preview
// before anything changes in the cluster.
func Validate(releases []Release, reg Registry) error {
	var errs []error
	for _, rel := range releases {
		if rel.Image == nil {
			continue
		}
		img := rel.Image
		fail := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("release %q: "+format, append([]any{rel.Name}, args...)...))
		}
		if img.Repository == "" {
			fail("image.repository is required")
		} else if strings.Contains(img.Repository, "@") ||
			strings.Contains(img.Repository[strings.LastIndex(img.Repository, "/")+1:], ":") {
			fail("image.repository %q must not contain a tag or digest", img.Repository)
		}
		if img.Tag == "" {
			fail("image.tag is required")
		}
		if img.Digest != "" && !digestPattern.MatchString(img.Digest) {
			fail("image.digest %q must match sha256:<64 hex>", img.Digest)
		}
	}

	var missing []string
	for _, key := range []struct {
		name string
		set  bool
	}{
		{"metrik:registryServer", reg.Server != ""},
		{"metrik:registryUsername", reg.Username != ""},
		{"metrik:registryPassword", reg.PasswordSet},
	} {
		if !key.set {
			missing = append(missing, key.name)
		}
	}
	switch {
	case len(missing) > 0 && len(missing) < 3:
		errs = append(errs, fmt.Errorf("metrik:registryServer, metrik:registryUsername and "+
			"metrik:registryPassword must be set together; missing %s", strings.Join(missing, ", ")))
	case len(missing) == 0 && reg.ImagePullSecret != "":
		errs = append(errs, errors.New("set either the registry credentials or "+
			"metrik:imagePullSecret, not both"))
	}

	return errors.Join(errs...)
}

// ImageTag returns the chart's image.tag value: the tag, or tag@digest when pinned.
func ImageTag(img Image) string {
	if img.Digest == "" {
		return img.Tag
	}
	return img.Tag + "@" + img.Digest
}

// Values returns the chart values that override the values file, or nil when there is nothing
// to override.
func Values(img *Image, pullSecret string) map[string]any {
	values := map[string]any{}
	if img != nil {
		values["image"] = map[string]any{"repository": img.Repository, "tag": ImageTag(*img)}
	}
	if pullSecret != "" {
		values["imagePullSecrets"] = []any{map[string]any{"name": pullSecret}}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

// DockerConfigJSON returns the .dockerconfigjson content of a pull secret for one registry.
func DockerConfigJSON(server, username, password string) (string, error) {
	type auth struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Auth     string `json:"auth"`
	}
	config := map[string]map[string]auth{"auths": {server: {
		Username: username,
		Password: password,
		Auth:     base64.StdEncoding.EncodeToString([]byte(username + ":" + password)),
	}}}
	b, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
