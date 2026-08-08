// Package scaffold writes the files a new knowledge store needs to exist: the
// bundle declaration, a frontmatter schema, an empty packages file, a root
// index, and a template to copy from.
//
// The result validates and builds as it stands, so `qm bundle init` produces a
// store that is already well-formed rather than a pile of files an author must
// first repair.
package scaffold

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mberwanger/quartermaster/internal/index"
)

//go:embed all:files
var files embed.FS

// ConfigFile is the declaration whose presence marks a directory as a store.
const ConfigFile = "bundle.yaml"

const (
	nameToken        = "{{NAME}}"
	encodedNameToken = "{{NAME_JSON}}"
	okfVersionToken  = "{{OKF_VERSION}}"
)

// ValidateName checks the store name before any scaffold files are written.
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("store name must not be empty")
	}
	if strings.ContainsAny(name, "\r\n") {
		return fmt.Errorf("store name must fit on one line")
	}
	return nil
}

// ownedRelPaths returns the store-relative paths the scaffold writes, in a
// stable order. It is the single source of truth for what counts as
// "scaffold-owned": Exists and Write both derive from it, so they can never
// disagree about which files --force governs.
func ownedRelPaths() ([]string, error) {
	var relPaths []string
	err := fs.WalkDir(files, "files", func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel, err := filepath.Rel("files", p)
		if err != nil {
			return err
		}
		relPaths = append(relPaths, filepath.ToSlash(rel))
		return nil
	})
	return relPaths, err
}

// ExistingOwnedFile reports the first scaffold-owned path already present in
// dir, if any, so a caller can say specifically what it found rather than
// assuming it was bundle.yaml.
func ExistingOwnedFile(dir string) (string, bool) {
	relPaths, err := ownedRelPaths()
	if err != nil {
		// The scaffold is embedded in the binary, so a walk failure here means
		// the binary is broken, not that dir holds a store.
		return "", false
	}
	for _, rel := range relPaths {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err == nil {
			return rel, true
		}
	}
	return "", false
}

// Exists reports whether dir already holds any scaffold-owned file. A caller
// uses this to require --force before Write replaces an existing store,
// rather than checking only for bundle.yaml and missing the rest.
func Exists(dir string) bool {
	_, found := ExistingOwnedFile(dir)
	return found
}

// Write scaffolds a store into dir, substituting the store name, and returns
// the files it wrote in order. Existing scaffold-owned files are replaced;
// callers decide whether that is allowed before invoking Write.
//
// Every file is first rendered into a temporary staging directory inside dir.
// Only once every file has been generated and no destination path conflicts
// with a directory does Write start moving files into place, so a failure
// partway through content generation or a blocked destination never leaves
// dir with a partial scaffold.
func Write(dir, name string) ([]string, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	encodedName, err := json.Marshal(name)
	if err != nil {
		return nil, fmt.Errorf("encode store name: %w", err)
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("scaffold %s: %w", dir, err)
	}
	staging, err := os.MkdirTemp(dir, ".qm-init-*")
	if err != nil {
		return nil, fmt.Errorf("stage scaffold: %w", err)
	}
	defer os.RemoveAll(staging)

	relPaths, err := stageFiles(staging, name, encodedName)
	if err != nil {
		return nil, err
	}
	if err := checkNoDirectoryCollisions(dir, relPaths); err != nil {
		return nil, err
	}
	if err := commitStaged(dir, staging, relPaths); err != nil {
		return nil, err
	}
	return relPaths, nil
}

// stageFiles renders every embedded scaffold file, with the store name
// substituted, into staging and returns the relative paths written.
func stageFiles(staging, name string, encodedName []byte) ([]string, error) {
	var relPaths []string

	err := fs.WalkDir(files, "files", func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}

		rel, err := filepath.Rel("files", p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		body, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		body = bytes.ReplaceAll(body, []byte(nameToken), []byte(name))
		body = bytes.ReplaceAll(body, []byte(encodedNameToken), encodedName)
		body = bytes.ReplaceAll(body, []byte(okfVersionToken), []byte(index.CurrentOKFVersion))

		stagePath := filepath.Join(staging, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(stagePath), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(stagePath, body, 0o644); err != nil { //nolint:gosec // store source is committed, not a secret
			return err
		}
		relPaths = append(relPaths, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return relPaths, nil
}

// checkNoDirectoryCollisions fails before any file is moved into dir if any
// scaffold-owned path cannot end up where it needs to be: either the
// destination itself is occupied by a directory, which a rename could not
// replace, or one of its ancestor directories is occupied by a plain file,
// which MkdirAll could never turn into a directory. Both are checked up
// front, over every path, so a collision on the last file in the list can
// never surface after earlier files have already been committed.
func checkNoDirectoryCollisions(dir string, relPaths []string) error {
	for _, rel := range relPaths {
		if err := checkPathIsWritable(dir, rel); err != nil {
			return err
		}
	}
	return nil
}

// checkPathIsWritable walks rel one path segment at a time, so a file
// blocking an intermediate directory (e.g. "meta" as a plain file, blocking
// "meta/packages.yaml") is caught here rather than surfacing as an
// os.MkdirAll failure partway through commitStaged.
func checkPathIsWritable(dir, rel string) error {
	segments := strings.Split(rel, "/")
	for i, segment := range segments {
		partial := filepath.Join(dir, filepath.Join(segments[:i+1]...))
		info, err := os.Lstat(partial)
		if err != nil {
			continue // does not exist yet; MkdirAll or Rename will create it
		}

		atLeaf := i == len(segments)-1
		if atLeaf && info.IsDir() {
			return fmt.Errorf("scaffold %s: a directory already exists at that path", rel)
		}
		if !atLeaf && !info.IsDir() {
			return fmt.Errorf("scaffold %s: %s already exists and is not a directory", rel, segment)
		}
	}
	return nil
}

// commitStaged moves every staged file into its final location under dir.
// Staging and dir share a filesystem, since staging is created inside dir, so
// each move is a rename rather than a copy.
func commitStaged(dir, staging string, relPaths []string) error {
	for _, rel := range relPaths {
		dest := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
			return fmt.Errorf("scaffold %s: %w", rel, err)
		}
		if err := os.Rename(filepath.Join(staging, filepath.FromSlash(rel)), dest); err != nil {
			return fmt.Errorf("scaffold %s: %w", rel, err)
		}
	}
	return nil
}
