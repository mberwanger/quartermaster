// Package provider resolves a bundle source to an in-memory bundle.
//
// Every provider yields the same thing: a bundle plus a content digest. Nothing
// downstream of resolution knows which scheme produced it, so a consumer treats
// an OCI artifact, a local directory, a git tree, and a tarball identically.
//
// Remote schemes cache by digest under the per-machine cache, so many
// repositories on the same bundle pull it once. file:// is deliberately not
// cached: a local tree is the authoring loop, and every save must be visible.
package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/preflight"
)

// Auth carries credentials for a remote source. Its zero value means none, and
// each scheme then falls back to whatever ambient credentials the underlying
// tool already uses: the Docker store for oci, git's own credentials for git,
// and nothing for https.
type Auth struct {
	// Token is a bearer token: an OCI access token, an https Authorization
	// bearer, or a git http.extraHeader bearer.
	Token string
	// Username and Password are basic credentials, for a registry or an https
	// or git endpoint that wants them.
	Username string
	Password string
}

func (a Auth) set() bool {
	return a.Token != "" || a.Username != "" || a.Password != ""
}

// Resolve turns a source URL into a bundle. Relative file:// paths resolve
// against baseDir, the directory of the manifest that named the source. auth is
// used for remote schemes and ignored for file://.
func Resolve(source, baseDir string, auth Auth) (*bundle.Bundle, error) {
	scheme, rest, ok := strings.Cut(source, "://")
	if !ok {
		return nil, fmt.Errorf("source %q has no scheme, expected e.g. file://path", source)
	}

	switch scheme {
	case "file":
		return resolveFile(rest, baseDir)
	case "oci":
		return resolveOCI(rest, auth)
	case "https":
		return resolveHTTPS(source, auth)
	case "git+https":
		return resolveGit("https://"+rest, auth)
	default:
		return nil, fmt.Errorf("unknown source scheme %q", scheme)
	}
}

// resolveFile reads a local directory in place, never cached.
func resolveFile(path, baseDir string) (*bundle.Bundle, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return load(path)
}

// load reads a directory as a bundle: a built artifact if it carries meta.json,
// or a store tree built on the fly if it carries bundle.yaml. Building a source
// tree on the fly is what closes the authoring loop — edit a rule in the store,
// sync in a consuming repository, see the result with no publish step between.
//
// A tarball or a git tree often nests the bundle one directory down, so a single
// unambiguous subdirectory is followed before giving up.
func load(dir string) (*bundle.Bundle, error) {
	if b, ok, err := loadHere(dir); ok || err != nil {
		return b, err
	}

	if nested, ok := singleSubdir(dir); ok {
		if b, ok, err := loadHere(nested); ok || err != nil {
			return b, err
		}
	}

	return nil, fmt.Errorf("%s holds neither %s (a built bundle) nor %s (a store)",
		dir, bundle.MetaName, config.FileName)
}

// loadHere reports whether dir is itself a bundle or a store, and loads it.
func loadHere(dir string) (*bundle.Bundle, bool, error) {
	if exists(filepath.Join(dir, bundle.MetaName)) {
		b, err := bundle.Read(dir)
		return b, true, err
	}
	if exists(filepath.Join(dir, config.FileName)) {
		b, err := build(dir)
		return b, true, err
	}
	return nil, false, nil
}

// singleSubdir returns the only subdirectory of dir, when there is exactly one.
func singleSubdir(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var found string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if found != "" {
			return "", false
		}
		found = filepath.Join(dir, e.Name())
	}
	return found, found != ""
}

// build compiles a source tree in memory, the same code path qm bundle build runs.
func build(root string) (*bundle.Bundle, error) {
	result, err := preflight.Run(preflight.Options{Root: root})
	if err != nil {
		return nil, err
	}
	return result.Bundle, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
