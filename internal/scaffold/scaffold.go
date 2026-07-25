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
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:files
var files embed.FS

// ConfigFile is the declaration whose presence marks a directory as a store.
const ConfigFile = "bundle.yaml"

const nameToken = "{{NAME}}"

// Exists reports whether dir already holds a store.
func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ConfigFile))
	return err == nil
}

// Write scaffolds a store into dir, substituting the store name, and returns the
// files it created in the order they were written. It does not overwrite: a
// caller checks Exists first and decides whether to proceed.
func Write(dir, name string) ([]string, error) {
	var written []string

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

		dest := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(dest, body, 0o644); err != nil { //nolint:gosec // store source is committed, not a secret
			return err
		}
		written = append(written, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return written, nil
}
