package bundle

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"
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
	if err := checkFormat(b.Meta.Format); err != nil {
		return nil, err
	}
	if err := readJSON(fsys, CatalogName, &b.Catalog); err != nil {
		return nil, err
	}
	// Write always emits packages.json, so past the format check its absence
	// means a damaged artifact rather than a bundle that offers nothing. Reading
	// it as no packages would sync a repository to an empty set and call that
	// success, which is the failure checkFormat exists to prevent.
	if err := readJSON(fsys, PackagesName, &b.Packages); err != nil {
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

// formatVersion matches the major.minor form the artifact format declares.
var formatVersion = regexp.MustCompile(`^(\d+)\.(\d+)$`)

// checkFormat rejects an artifact this build cannot read.
//
// The match is exact, because before 1.0 a minor bump is allowed to change the
// layout and 0.3 to 0.4 did exactly that: it replaced rulesets.json with
// packages.json. Accepting a near miss is how a reader ends up returning
// something structurally valid and semantically empty.
//
// That was the behavior until this check existed. Nothing compared the declared
// format to the one this build writes, so reading an artifact whose layout had
// moved found no file where it looked, reported no error, and handed back a
// bundle with nothing in it. A repository would sync to an empty set and be told
// it had worked. The direction of the mismatch decides who can fix it, so the
// message says which way it went rather than only that it happened.
func checkFormat(declared string) error {
	if declared == Version {
		return nil
	}
	if declared == "" {
		return fmt.Errorf("artifact declares no format version; qm writes %s, so this was not built by qm bundle build", Version)
	}

	m := formatVersion.FindStringSubmatch(declared)
	if m == nil {
		return fmt.Errorf("artifact declares format %q, which is not a major.minor version; qm reads %s", declared, Version)
	}

	if newer(m, Version) {
		return fmt.Errorf("artifact is format %s and this qm reads %s; upgrade qm", declared, Version)
	}
	return fmt.Errorf("artifact is format %s and this qm reads %s; rebuild it with qm bundle build", declared, Version)
}

// newer reports whether a parsed major.minor is ahead of the current version,
// which is what separates "upgrade the tool" from "rebuild the artifact".
func newer(declared []string, current string) bool {
	cur := formatVersion.FindStringSubmatch(current)
	if cur == nil {
		return false
	}
	dMajor, _ := strconv.Atoi(declared[1])
	dMinor, _ := strconv.Atoi(declared[2])
	cMajor, _ := strconv.Atoi(cur[1])
	cMinor, _ := strconv.Atoi(cur[2])

	if dMajor != cMajor {
		return dMajor > cMajor
	}
	return dMinor > cMinor
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
