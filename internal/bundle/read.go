package bundle

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Read reconstructs a Bundle from a written artifact directory. It is the
// inverse of Write, so a bundle resolved from a provider that yields a built
// artifact (an OCI layer, an unpacked tarball, a dist directory) becomes the
// same in-memory Bundle a fresh Build produces.
//
// The digest is read from meta.json and trusted. A provider that cares about
// integrity verifies the digest against the source it resolved from before
// handing the directory here; recomputing it would require rebuilding the store
// tree's contribution and is the provider's concern, not the reader's.
func Read(dir string) (*Bundle, error) {
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	fsys := r.FS()

	b := &Bundle{}

	if err := readJSON(fsys, MetaName, &b.Meta); err != nil {
		return nil, err
	}
	if err := readJSON(fsys, CatalogName, &b.Catalog); err != nil {
		return nil, err
	}
	// packages.json is absent in an older bundle; treat that as no packages
	// rather than an error, so the failure is "this bundle offers nothing"
	// rather than an unreadable artifact.
	if err := readJSON(fsys, PackagesName, &b.Packages); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	b.Files, err = readTree(fsys, StoreDir)
	if err != nil {
		return nil, err
	}
	b.Controls, err = readTree(fsys, ControlsDir)
	if err != nil {
		return nil, err
	}

	return b, nil
}

// readTree reads every file under a subdirectory, returning paths relative to
// that subdirectory so they match what Build produced. A missing subdirectory is
// not an error: a bundle may carry no controls, or no store tree.
func readTree(fsys fs.FS, dir string) ([]File, error) {
	if _, err := fs.Stat(fsys, dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var files []File
	err := fs.WalkDir(fsys, dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		body, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		files = append(files, File{Path: strings.TrimPrefix(p, dir+"/"), Body: body})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func readJSON(fsys fs.FS, name string, v any) error {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
