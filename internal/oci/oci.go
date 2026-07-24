// Package oci pushes and pulls bundles as OCI artifacts.
//
// A bundle travels as a single deterministic tarball layer under a custom media
// type, wrapped in an artifact manifest whose annotations carry the source
// repository, the source commit, and the bundle's own content digest. The
// registry's digest addresses the artifact; the bundle digest in the annotations
// addresses the content, and the two are deliberately separate — republishing
// identical content from a different commit keeps the same bundle digest.
package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"

	"github.com/mberwanger/quartermaster/internal/archive"
)

// Media types and annotations that identify a Quartermaster bundle in a registry.
const (
	ArtifactType   = "application/vnd.quartermaster.bundle.v1+json"
	LayerMediaType = "application/vnd.quartermaster.bundle.v1.tar+gzip"

	// AnnotationBundleDigest carries the bundle's own content digest, which is
	// what a manifest pins and what survives a repack.
	AnnotationBundleDigest = "io.quartermaster.bundle.digest"
	// AnnotationRulesets lists the ruleset names the bundle compiled.
	AnnotationRulesets = "io.quartermaster.bundle.rulesets"
)

// layerTitle names the layer, so `oras pull` writes a sensible filename.
const layerTitle = "bundle.tar.gz"

// Auth carries explicit registry credentials. Its zero value means none, which
// falls back to the ambient Docker credential store.
type Auth struct {
	// AccessToken is a bearer token, used directly against the registry.
	AccessToken string
	// Username and Password are basic credentials, as `docker login` stores.
	Username string
	Password string
}

func (a Auth) set() bool {
	return a.AccessToken != "" || a.Username != "" || a.Password != ""
}

// Repo is a handle on one repository in a registry.
type Repo struct {
	repo *remote.Repository
	ref  registry.Reference
}

// Open parses a reference like ghcr.io/org/knowledge:v0.14.2 (or @sha256:...)
// and prepares a client for it, authenticated by the given credentials or, when
// none are given, by the ambient Docker credential store.
func Open(ref string, creds Auth) (*Repo, error) {
	parsed, err := registry.ParseReference(ref)
	if err != nil {
		return nil, fmt.Errorf("parse reference %q: %w", ref, err)
	}

	repo, err := remote.NewRepository(parsed.Registry + "/" + parsed.Repository)
	if err != nil {
		return nil, err
	}
	repo.Client = newClient(parsed.Registry, creds)
	if isLoopback(parsed.Registry) {
		// A registry on loopback is a local test registry, which is normally
		// served without TLS.
		repo.PlainHTTP = true
	}

	return &Repo{repo: repo, ref: parsed}, nil
}

// Reference returns the tag or digest the handle was opened with, defaulting to
// latest when the reference named none.
func (r *Repo) Reference() string {
	if r.ref.Reference == "" {
		return "latest"
	}
	return r.ref.Reference
}

// Resolve looks up the artifact descriptor for the handle's reference. It is a
// cheap call, which is what lets a caller check the cache before pulling.
func (r *Repo) Resolve(ctx context.Context) (ocispec.Descriptor, error) {
	return r.repo.Resolve(ctx, r.Reference())
}

// Extract fetches the artifact's bundle layer and unpacks it into dst. The blob
// is verified against its descriptor digest as it is read, so a registry that
// returns the wrong bytes fails here rather than silently materializing.
func (r *Repo) Extract(ctx context.Context, desc ocispec.Descriptor, dst string) error {
	manifestBytes, err := content.FetchAll(ctx, r.repo, desc)
	if err != nil {
		return fmt.Errorf("fetch manifest: %w", err)
	}

	var m ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	layer, err := bundleLayer(m)
	if err != nil {
		return err
	}

	blob, err := content.FetchAll(ctx, r.repo, layer)
	if err != nil {
		return fmt.Errorf("fetch bundle layer: %w", err)
	}
	return archive.UntarGz(blob, dst)
}

// Annotations returns the artifact manifest's annotations, which is how a caller
// reads the bundle digest and provenance without unpacking the layer.
func (r *Repo) Annotations(ctx context.Context, desc ocispec.Descriptor) (map[string]string, error) {
	manifestBytes, err := content.FetchAll(ctx, r.repo, desc)
	if err != nil {
		return nil, err
	}
	var m ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return nil, err
	}
	return m.Annotations, nil
}

// Push packs dir into a deterministic tarball, pushes it as the artifact's only
// layer, packs the manifest with the given annotations, and applies every tag.
func (r *Repo) Push(ctx context.Context, dir string, annotations map[string]string, tags []string) (ocispec.Descriptor, error) {
	blob, err := archive.TarGz(dir)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	layer, err := oras.PushBytes(ctx, r.repo, LayerMediaType, blob)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("push layer: %w", err)
	}
	layer.Annotations = map[string]string{ocispec.AnnotationTitle: layerTitle}

	manifest, err := oras.PackManifest(ctx, r.repo, oras.PackManifestVersion1_1, ArtifactType, oras.PackManifestOptions{
		Layers:              []ocispec.Descriptor{layer},
		ManifestAnnotations: annotations,
	})
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pack manifest: %w", err)
	}

	for _, tag := range tags {
		if tag == "" || strings.HasPrefix(tag, "sha256:") {
			continue
		}
		if err := r.repo.Tag(ctx, manifest, tag); err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("tag %s: %w", tag, err)
		}
	}
	return manifest, nil
}

// bundleLayer picks the bundle layer out of a manifest, preferring the declared
// media type and falling back to a lone layer so an artifact packed by another
// tool still resolves.
func bundleLayer(m ocispec.Manifest) (ocispec.Descriptor, error) {
	for _, l := range m.Layers {
		if l.MediaType == LayerMediaType {
			return l, nil
		}
	}
	if len(m.Layers) == 1 {
		return m.Layers[0], nil
	}
	return ocispec.Descriptor{}, fmt.Errorf("artifact carries no %s layer", LayerMediaType)
}

// newClient builds a client for one registry. Explicit credentials win; without
// them it reads the Docker credential store; without that it is anonymous, which
// is correct for a public registry, so a missing store is not an error.
func newClient(registryHost string, creds Auth) remote.Client {
	c := &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
	}
	switch {
	case creds.set():
		c.Credential = auth.StaticCredential(registryHost, auth.Credential{
			Username:    creds.Username,
			Password:    creds.Password,
			AccessToken: creds.AccessToken,
		})
	default:
		if store, err := credentials.NewStoreFromDocker(credentials.StoreOptions{}); err == nil {
			c.Credential = credentials.Credential(store)
		}
	}
	return c
}

// isLoopback reports whether host names the local machine. The host arrives as
// a reference's registry part, so it may carry a port, and an IPv6 address is
// bracketed there ("[::1]:5000") as a URL authority requires.
func isLoopback(host string) bool {
	h := host
	if hostOnly, _, err := net.SplitHostPort(h); err == nil {
		h = hostOnly
	}
	// An unbracketed bare "::1" fails SplitHostPort, so strip brackets after.
	h = strings.Trim(h, "[]")

	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
