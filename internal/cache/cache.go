// Package cache stores resolved bundles on disk, keyed by content digest.
//
// Twenty-one repositories on the same bundle should pull it once. Because the
// key is a digest, a cache hit is exact rather than a guess about freshness, and
// a populated entry never needs revalidating.
package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dirName is the per-machine cache root under the user's home directory.
const dirName = ".quartermaster"

// Root returns the cache root, creating nothing. QM_CACHE_DIR overrides it,
// which is what tests and CI use to stay off a developer's real cache.
func Root() (string, error) {
	if override := os.Getenv("QM_CACHE_DIR"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dirName, "cache"), nil
}

// Populate returns the directory holding the entry for key, filling it first if
// it is not already present.
//
// fill writes into a temporary directory that is renamed into place only after
// it succeeds, so a failed or interrupted fetch never leaves a half-populated
// entry that a later run would trust.
func Populate(kind, key string, fill func(dir string) error) (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(root, kind, sanitize(key))
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return "", err
	}

	tmp, err := os.MkdirTemp(parent, ".tmp-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if err := fill(tmp); err != nil {
		return "", err
	}

	if err := os.Rename(tmp, dir); err != nil {
		// A concurrent process may have populated the same entry first, which is
		// a win rather than a race: the content is identical by construction.
		if _, statErr := os.Stat(dir); statErr == nil {
			return dir, nil
		}
		return "", fmt.Errorf("commit cache entry: %w", err)
	}
	return dir, nil
}

// sanitize turns a digest or URL into a safe single path segment.
func sanitize(key string) string {
	r := strings.NewReplacer(":", "-", "/", "_", "\\", "_", " ", "_")
	return r.Replace(key)
}
