package bundle

import (
	"encoding/json"
	"os"
	"path"
	"strings"
)

// Layout inside a written artifact.
const (
	MetaName     = "meta.json"
	CatalogName  = "catalog.json"
	PackagesName = "packages.json"
	StoreMDName  = "store.md"
	StoreDir     = "store"
	// ControlsDir holds fixtures the review and audit jobs read and no agent
	// grounds on. It sits outside StoreDir so that an agent pointed at the
	// store cannot reach it, whether it reads files or the concatenated view.
	ControlsDir = "controls"
)

// Write emits the artifact into dir, creating it if needed.
//
// The directory is cleared first, so a file whose doc was deleted cannot survive
// into a later build and go on being read.
//
// Everything below the root is written through os.Root, matching how the store
// is read. A path in the tree comes from the store itself, and one that
// traversed upward would be a write outside the artifact.
func Write(b *Bundle, dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	r, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	if err := writeJSON(r, MetaName, b.Meta); err != nil {
		return err
	}
	if err := writeJSON(r, CatalogName, b.Catalog); err != nil {
		return err
	}
	if err := writeJSON(r, PackagesName, b.Packages); err != nil {
		return err
	}
	if err := r.WriteFile(StoreMDName, []byte(b.StoreMD), 0o600); err != nil {
		return err
	}

	for dir, set := range map[string][]File{StoreDir: b.Files, ControlsDir: b.Controls} {
		if len(set) == 0 {
			continue
		}
		if err := mkdirAll(r, dir); err != nil {
			return err
		}
		for _, f := range set {
			p := path.Join(dir, f.Path)
			if err := mkdirAll(r, path.Dir(p)); err != nil {
				return err
			}
			if err := r.WriteFile(p, f.Body, 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

// mkdirAll creates dir and its parents inside the root. os.Root has no recursive
// variant, so the ancestors are walked from the top down.
func mkdirAll(r *os.Root, dir string) error {
	if dir == "." || dir == "/" {
		return nil
	}

	var built string
	for part := range strings.SplitSeq(dir, "/") {
		if part == "" || part == "." {
			continue
		}
		built = path.Join(built, part)
		if err := r.Mkdir(built, 0o750); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func writeJSON(r *os.Root, name string, v any) error {
	f, err := r.Create(path.Clean(name))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return f.Close()
}
