// Package archive packs and unpacks the tarball a bundle travels in.
//
// Packing is deterministic: entries are sorted by path and every timestamp,
// owner, and mode bit that does not affect content is normalized. Two packs of
// identical content therefore produce identical bytes, so republishing an
// unchanged bundle does not churn the registry with a new layer digest.
package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxEntrySize bounds a single unpacked file, so a malicious archive cannot
// exhaust the disk through one enormous entry.
const maxEntrySize = 1 << 30 // 1 GiB

// TarGz packs dir into a gzipped tarball with deterministic bytes.
func TarGz(dir string) ([]byte, error) {
	paths, err := walk(dir)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	// No compression header timestamp or name, for reproducibility.
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(zw)

	for _, rel := range paths {
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))) //nolint:gosec // rel comes from walking dir
		if err != nil {
			return nil, err
		}
		hdr := &tar.Header{
			Name:     rel,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
			// Zero time, uid, gid and no names: none of it is content.
			Format: tar.FormatPAX,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(body); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// walk returns every regular file under dir, slash-separated and sorted.
func walk(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() || !e.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// UntarGz unpacks a gzipped tarball into dst.
//
// An entry whose path escapes dst is rejected rather than written. The archive
// comes off a network, and a bundle is unpacked into a developer's machine, so
// path traversal is the one thing this must not get wrong.
func UntarGz(data []byte, dst string) error {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()

	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			// Directories are implied by file paths; links and devices have no
			// place in a bundle and are refused by omission.
			continue
		}

		target, err := safeJoin(dst, hdr.Name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}

		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, io.LimitReader(tr, maxEntrySize)); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
}

// safeJoin resolves name under dst, refusing anything that would land outside.
func safeJoin(dst, name string) (string, error) {
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("archive entry %q is an absolute path", name)
	}
	target := filepath.Join(dst, filepath.FromSlash(name))
	rel, err := filepath.Rel(dst, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry %q escapes the destination", name)
	}
	return target, nil
}
