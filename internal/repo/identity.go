package repo

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// gitName is the entry every working tree has, a directory in a main worktree
// and a file pointing elsewhere in a linked one.
const gitName = ".git"

// Identity returns a stable name for the repository at dir: the same string
// from every worktree of that repository, and from every machine that has it.
//
// A working-tree path is not that name. Worktrees exist so several sessions can
// run at once, so keying on the path turns one repository into three. The
// cross-repository spread that promotion is judged on then splits into
// singletons, and a document opened in every checkout of a single repository
// reads as a trend across three. Neither can be corrected after the fact,
// because the recorded value no longer says which repository it meant.
//
// The remote is the only name every worktree and every developer already agrees
// on, so it is preferred. The shared git directory is the fallback, because it
// too is common to every worktree. A directory that is neither falls back to its
// own name, which is still stable for the one checkout it describes.
func Identity(dir string) string {
	common, ok, _ := commonDir(dir)
	if !ok {
		return strings.ToLower(baseName(dir))
	}

	if url, ok := remoteURL(filepath.Join(common, "config")); ok {
		if id := remoteIdentity(url); id != "" {
			return id
		}
	}

	// No remote to be named by. The main working tree's own name is shared by
	// every linked worktree, which is the property that matters here.
	if filepath.Base(common) == gitName {
		return strings.ToLower(baseName(filepath.Dir(common)))
	}
	return strings.ToLower(strings.TrimSuffix(baseName(common), ".git"))
}

// Branch returns the branch checked out at dir, or the empty string when the
// head is detached or dir is not a working tree.
//
// It reads the worktree's own git directory rather than the shared one, because
// the branch is the one piece of git state a worktree does not share, and it is
// most of the reason to have several.
func Branch(dir string) string {
	git, ok, err := gitDir(dir)
	if err != nil || !ok {
		return ""
	}

	raw, err := os.ReadFile(filepath.Join(git, "HEAD")) //nolint:gosec // path derived from the repository's own .git
	if err != nil {
		return ""
	}
	ref, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "ref:")
	if !ok {
		return "" // detached: a sha names no branch
	}
	return strings.TrimPrefix(strings.TrimSpace(ref), "refs/heads/")
}

// baseName is filepath.Base against an absolute path, so that "." names the
// directory rather than itself.
func baseName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return filepath.Base(abs)
}

// gitDir resolves the git directory for dir, handling both a normal .git
// directory and a .git file that points elsewhere, as a linked worktree or a
// submodule has.
func gitDir(dir string) (string, bool, error) {
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

// remoteURL returns a remote's url from a git config file: origin's when there
// is one, and otherwise the first remote in the file, so a repository whose
// remote is named something else is still identified rather than falling back to
// a directory name.
func remoteURL(path string) (string, bool) {
	f, err := os.Open(path) //nolint:gosec // path derived from the repository's own .git
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()

	var section, origin, first string

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "["):
			section, _ = remoteSection(line)
			continue
		case section == "":
			continue
		}

		key, val, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "url" {
			continue
		}
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		if section == "origin" && origin == "" {
			origin = val
		}
		if first == "" {
			first = val
		}
	}

	if origin != "" {
		return origin, true
	}
	return first, first != ""
}

// remoteSection returns the remote a section header names, and the empty string
// for any other section.
func remoteSection(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, `[remote "`)
	if !ok {
		return "", false
	}
	name, _, ok := strings.Cut(rest, `"`)
	if !ok {
		return "", false
	}
	return name, true
}

// remoteIdentity reduces a remote url to host and path, dropping the scheme, any
// credentials, the port, and a .git suffix, so the ssh and https spellings of
// one repository reduce to the same name. A remote that is a local path keeps
// only its last element, since the rest is that machine's layout.
func remoteIdentity(url string) string {
	u := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(url), "/"), ".git")
	if u == "" {
		return ""
	}

	switch host, rest, ok := strings.Cut(u, ":"); {
	case strings.Contains(u, "://"):
		_, u, _ = strings.Cut(u, "://")
	case ok && strings.ContainsAny(host, "@."):
		// scp-like, as in git@github.com:owner/name.
		u = host + "/" + strings.TrimPrefix(rest, "/")
	default:
		return strings.ToLower(strings.TrimSuffix(baseName(u), ".git"))
	}

	if _, after, ok := strings.Cut(u, "@"); ok {
		u = after
	}
	host, path, ok := strings.Cut(u, "/")
	if !ok || path == "" {
		return ""
	}
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	return strings.ToLower(host + "/" + strings.Trim(path, "/"))
}
