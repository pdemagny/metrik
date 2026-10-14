package stack

import (
	"reflect"
	"strings"
	"testing"
)

const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestValidate(t *testing.T) {
	t.Parallel()
	created := Registry{Server: "ghcr.io", Username: "u", PasswordSet: true}

	tests := []struct {
		name     string
		releases []Release
		registry Registry
		wantErr  []string // substrings of the error; nil means valid
	}{
		{
			name:     "no image, no registry",
			releases: []Release{{Name: "metrik"}},
		},
		{
			name:     "full image with digest, credentials created",
			releases: []Release{{Name: "metrik", Image: &Image{Repository: "ghcr.io/x/metrik", Tag: "0.1.0", Digest: digest}}},
			registry: created,
		},
		{
			name:     "registry with port",
			releases: []Release{{Name: "metrik", Image: &Image{Repository: "localhost:5000/metrik", Tag: "sha-1a2b3c4"}}},
		},
		{
			name:     "existing pull secret",
			releases: []Release{{Name: "metrik"}},
			registry: Registry{ImagePullSecret: "my-secret"},
		},
		{
			name:     "V1 missing repository",
			releases: []Release{{Name: "metrik", Image: &Image{Tag: "0.1.0"}}},
			wantErr:  []string{`release "metrik"`, "image.repository"},
		},
		{
			name:     "V1 digest without repository",
			releases: []Release{{Name: "metrik", Image: &Image{Tag: "0.1.0", Digest: digest}}},
			wantErr:  []string{`release "metrik"`, "image.repository"},
		},
		{
			name:     "V2 missing tag",
			releases: []Release{{Name: "demo-api", Image: &Image{Repository: "ghcr.io/x/demo-api"}}},
			wantErr:  []string{`release "demo-api"`, "image.tag"},
		},
		{
			name:     "V3 repository with tag",
			releases: []Release{{Name: "metrik", Image: &Image{Repository: "ghcr.io/x/metrik:1.0", Tag: "1.0"}}},
			wantErr:  []string{`release "metrik"`, "image.repository"},
		},
		{
			name:     "V3 repository with digest",
			releases: []Release{{Name: "metrik", Image: &Image{Repository: "ghcr.io/x/metrik@" + digest, Tag: "1.0"}}},
			wantErr:  []string{`release "metrik"`, "image.repository"},
		},
		{
			name:     "V4 malformed digest",
			releases: []Release{{Name: "metrik", Image: &Image{Repository: "ghcr.io/x/metrik", Tag: "1.0", Digest: "sha256:abc"}}},
			wantErr:  []string{`release "metrik"`, "image.digest"},
		},
		{
			name:     "V5 password missing",
			registry: Registry{Server: "ghcr.io", Username: "u"},
			wantErr:  []string{"metrik:registryPassword"},
		},
		{
			name:     "V5 server and username missing",
			registry: Registry{PasswordSet: true},
			wantErr:  []string{"metrik:registryServer", "metrik:registryUsername"},
		},
		{
			name:     "V6 both modes",
			registry: Registry{Server: "ghcr.io", Username: "u", PasswordSet: true, ImagePullSecret: "my-secret"},
			wantErr:  []string{"metrik:imagePullSecret"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(tt.releases, tt.registry)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error mentioning %q", tt.wantErr)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Validate() = %q, want it to mention %q", err, want)
				}
			}
		})
	}
}

func TestMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		registry Registry
		want     Mode
	}{
		{"none", Registry{}, ModeNone},
		{"created", Registry{Server: "ghcr.io", Username: "u", PasswordSet: true}, ModeCreated},
		{"existing", Registry{ImagePullSecret: "my-secret"}, ModeExisting},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.registry.Mode(); got != tt.want {
				t.Errorf("Mode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestImageTag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		image Image
		want  string
	}{
		{"tag only", Image{Repository: "r", Tag: "0.1.0"}, "0.1.0"},
		{"tag and digest", Image{Repository: "r", Tag: "0.1.0", Digest: digest}, "0.1.0@" + digest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ImageTag(tt.image); got != tt.want {
				t.Errorf("ImageTag() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValues(t *testing.T) {
	t.Parallel()
	image := &Image{Repository: "ghcr.io/x/metrik", Tag: "0.1.0"}
	imageValues := map[string]any{"repository": "ghcr.io/x/metrik", "tag": "0.1.0"}
	pullSecrets := []any{map[string]any{"name": "registry-credentials"}}

	tests := []struct {
		name       string
		image      *Image
		pullSecret string
		want       map[string]any
	}{
		{"nothing", nil, "", nil},
		{"image only", image, "", map[string]any{"image": imageValues}},
		{"pull secret only", nil, "registry-credentials", map[string]any{"imagePullSecrets": pullSecrets}},
		{"both", image, "registry-credentials", map[string]any{"image": imageValues, "imagePullSecrets": pullSecrets}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Values(tt.image, tt.pullSecret); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Values() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDockerConfigJSON(t *testing.T) {
	t.Parallel()
	got, err := DockerConfigJSON("ghcr.io", "u", "p")
	if err != nil {
		t.Fatalf("DockerConfigJSON() error = %v", err)
	}
	want := `{"auths":{"ghcr.io":{"username":"u","password":"p","auth":"dTpw"}}}`
	if got != want {
		t.Errorf("DockerConfigJSON() = %s, want %s", got, want)
	}
}
