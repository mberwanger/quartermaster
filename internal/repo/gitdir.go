package repo

import (
	"os"
	"path/filepath"
	"strings"
)

// gitName is the entry every working tree has, a directory in a main worktree
// and a file pointing elsewhere in a linked one.
const gitName = ".git"

// gitDir resolves the git directory for dir, handling both a normal .git
// directory and a .git file that points elsewhere, as a linked worktree or a
// submodule has.
//
// It walks up, the way git itself does, because work happens in subdirectories.
// Without the walk a session that started in web/ belongs to a repository named
// "web", and one repository becomes as many repositories as it has directories
// people work in. That is the same fragmentation a worktree causes, arriving by
// a different route.
func gitDir(dir string) (string, bool, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false, err
	}

	for {
		found, ok, err := gitDirAt(abs)
		if err != nil || ok {
			return found, ok, err
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", false, nil
		}
		abs = parent
	}
}

// gitDirAt resolves the git directory declared by exactly this directory.
func gitDirAt(dir string) (string, bool, error) {
	p := filepath.Join(dir, gitName)
	fi, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if fi.IsDir() {
		return p, true, nil
	}

	raw, err := os.ReadFile(p) //nolint:gosec // path is the repository's own .git
	if err != nil {
		return "", false, err
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "gitdir:")
	if !ok {
		return "", false, nil
	}
	target = strings.TrimSpace(target)
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, target)
	}
	return filepath.Clean(target), true, nil
}

// commonDir resolves the git directory shared by every worktree of a
// repository. A linked worktree keeps its own git directory and names the shared
// one in a commondir file beside it; for a main worktree the git directory is
// already the shared one.
func commonDir(dir string) (string, bool, error) {
	git, ok, err := gitDir(dir)
	if err != nil || !ok {
		return "", false, err
	}

	raw, err := os.ReadFile(filepath.Join(git, "commondir")) //nolint:gosec // path derived from the repository's own .git
	if err != nil {
		// No commondir file means this is a main worktree, whose git directory
		// is already the shared one. Any other failure is real.
		if os.IsNotExist(err) {
			return git, true, nil
		}
		return "", false, err
	}
	p := strings.TrimSpace(string(raw))
	if p == "" {
		return git, true, nil
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(git, p)
	}
	return filepath.Clean(p), true, nil
}
