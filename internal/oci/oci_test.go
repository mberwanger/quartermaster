package oci

import (
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func layer(mediaType string) ocispec.Descriptor {
	return ocispec.Descriptor{MediaType: mediaType}
}

func TestBundleLayer(t *testing.T) {
	other := "application/vnd.oci.image.layer.v1.tar"

	tests := []struct {
		name    string
		layers  []ocispec.Descriptor
		want    string
		wantErr bool
	}{
		{
			name:   "declared media type",
			layers: []ocispec.Descriptor{layer(LayerMediaType)},
			want:   LayerMediaType,
		},
		{
			// An artifact packed by another tool still resolves, which is the
			// whole point of the fallback.
			name:   "lone layer of another type",
			layers: []ocispec.Descriptor{layer(other)},
			want:   other,
		},
		{
			// The declared type wins even when it is not first, so a signature
			// or attestation layer alongside the bundle cannot shadow it.
			name:   "declared media type among others",
			layers: []ocispec.Descriptor{layer(other), layer(LayerMediaType)},
			want:   LayerMediaType,
		},
		{
			// Ambiguous: several layers, none declared. Guessing would be worse
			// than failing.
			name:    "several layers none declared",
			layers:  []ocispec.Descriptor{layer(other), layer("application/json")},
			wantErr: true,
		},
		{
			name:    "no layers",
			layers:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bundleLayer(ocispec.Manifest{Layers: tt.layers})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got layer %q", got.MediaType)
				}
				return
			}
			if err != nil {
				t.Fatalf("bundleLayer: %v", err)
			}
			if got.MediaType != tt.want {
				t.Fatalf("got %q, want %q", got.MediaType, tt.want)
			}
		})
	}
}

// TestIsLoopback covers the forms a registry host actually arrives in. An IPv6
// address is bracketed in a reference, and that bracketed form is what a local
// registry on ::1 produces — it must still be recognized, or the client tries
// TLS against a plaintext test registry.
func TestIsLoopback(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"localhost:5000", true},
		{"127.0.0.1", true},
		{"127.0.0.1:5000", true},
		{"127.0.0.53:5000", true},
		{"::1", true},
		{"[::1]", true},
		{"[::1]:5000", true},
		{"ghcr.io", false},
		{"ghcr.io:443", false},
		{"registry.localhost.example.com", false},
		{"10.0.0.1:5000", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := isLoopback(tt.host); got != tt.want {
				t.Fatalf("isLoopback(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestOpenReference(t *testing.T) {
	const digest = "sha256:6b4f1c8e2a9d0f3b5c7e1a2d4f6890abcdef0123456789abcdef0123456789ab"

	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"tag", "ghcr.io/org/knowledge:v0.14.2", "v0.14.2"},
		{"digest", "ghcr.io/org/knowledge@" + digest, digest},
		{"no reference defaults to latest", "ghcr.io/org/knowledge", "latest"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Open(tt.ref, Auth{})
			if err != nil {
				t.Fatalf("Open(%q): %v", tt.ref, err)
			}
			if got := r.Reference(); got != tt.want {
				t.Fatalf("Reference() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenRejectsBadReference(t *testing.T) {
	for _, ref := range []string{"", "ghcr.io", "ghcr.io/Org/Knowledge", "ghcr.io/org/kn:bad tag"} {
		t.Run(ref, func(t *testing.T) {
			if _, err := Open(ref, Auth{}); err == nil {
				t.Fatalf("Open(%q) succeeded, want an error", ref)
			}
		})
	}
}

// TestOpenPlainHTTP pins the loopback rule at the level a caller sees it: a
// local registry is served without TLS, a remote one is not.
func TestOpenPlainHTTP(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"localhost:5000/org/knowledge:v1", true},
		{"127.0.0.1:5000/org/knowledge:v1", true},
		{"[::1]:5000/org/knowledge:v1", true},
		{"ghcr.io/org/knowledge:v1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			r, err := Open(tt.ref, Auth{})
			if err != nil {
				t.Fatalf("Open(%q): %v", tt.ref, err)
			}
			if got := r.repo.PlainHTTP; got != tt.want {
				t.Fatalf("PlainHTTP = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthSet(t *testing.T) {
	tests := []struct {
		name string
		auth Auth
		want bool
	}{
		{"zero value", Auth{}, false},
		{"access token", Auth{AccessToken: "t"}, true},
		{"username and password", Auth{Username: "u", Password: "p"}, true},
		// A username with no password is still an explicit credential: falling
		// back to the Docker store would silently ignore what the caller passed.
		{"username only", Auth{Username: "u"}, true},
		{"password only", Auth{Password: "p"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.auth.set(); got != tt.want {
				t.Fatalf("set() = %v, want %v", got, tt.want)
			}
		})
	}
}
