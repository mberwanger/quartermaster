package provider

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/cache"
)

// resolveGit clones a bundle out of a git repository.
//
// The source syntax is git+https://host/org/repo.git//subdir#ref, where //subdir
// and #ref are both optional. A store usually lives in a subdirectory of its
// repository, so naming it is the common case rather than an edge one.
//
// The ref is resolved to a commit before anything is fetched, and the commit is
// the cache key. A branch that has not moved therefore costs one ls-remote, and
// two repositories on the same commit clone once.
func resolveGit(source string, auth Auth) (*bundle.Bundle, error) {
	url, subdir, ref := parseGitSource(source)

	commit, err := gitResolve(url, ref, auth)
	if err != nil {
		return nil, err
	}

	dir, err := cache.Populate("git", commit, func(tmp string) error {
		return gitFetch(url, ref, tmp, auth)
	})
	if err != nil {
		return nil, err
	}

	root := dir
	if subdir != "" {
		root = filepath.Join(dir, filepath.FromSlash(subdir))
		if !exists(root) {
			return nil, fmt.Errorf("%s has no directory %q at %s", url, subdir, commit[:min(len(commit), 12)])
		}
	}
	return load(root)
}

// parseGitSource splits a git source into its URL, optional subdirectory, and
// optional ref.
func parseGitSource(source string) (url, subdir, ref string) {
	url = source
	if u, r, ok := strings.Cut(url, "#"); ok {
		url, ref = u, r
	}
	// The subdirectory separator is "//" after the scheme's own "://".
	if i := strings.Index(url[len("https://"):], "//"); i >= 0 {
		at := len("https://") + i
		url, subdir = url[:at], strings.Trim(url[at+2:], "/")
	}
	return url, subdir, ref
}

// gitResolve turns a ref into a commit sha without cloning.
func gitResolve(url, ref string, auth Auth) (string, error) {
	target := ref
	if target == "" {
		target = "HEAD"
	}

	out, err := runGit("", auth, "ls-remote", url, target)
	if err != nil {
		return "", err
	}

	line := strings.TrimSpace(out)
	if line == "" {
		// ls-remote prints nothing for a ref it cannot see. A full commit sha is
		// a legitimate ref that ls-remote will not match, so accept one as-is
		// and let the fetch decide whether the server will serve it.
		if isCommitSHA(ref) {
			return ref, nil
		}
		return "", fmt.Errorf("%s has no ref %q", url, target)
	}

	sha, _, _ := strings.Cut(line, "\t")
	return strings.TrimSpace(sha), nil
}

// gitFetch shallow-fetches a single ref into dir and strips the git metadata,
// leaving only the tree. Init-and-fetch is used rather than clone because it
// works uniformly for a branch, a tag, and (where the server allows it) a commit.
func gitFetch(url, ref, dir string, auth Auth) error {
	target := ref
	if target == "" {
		target = "HEAD"
	}

	steps := [][]string{
		{"init", "--quiet"},
		{"remote", "add", "origin", url},
		{"fetch", "--quiet", "--depth", "1", "origin", target},
		{"checkout", "--quiet", "FETCH_HEAD"},
	}
	for _, args := range steps {
		if _, err := runGit(dir, auth, args...); err != nil {
			return err
		}
	}

	// The tree is the content; the history is not part of the bundle.
	return os.RemoveAll(filepath.Join(dir, ".git"))
}

func runGit(dir string, auth Auth, args ...string) (string, error) {
	cmd := exec.Command("git", args...) //nolint:gosec // args are constructed here, not user shell input
	if dir != "" {
		cmd.Dir = dir
	}
	// Never prompt: a credential prompt in a sync would hang a git hook.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	// A token is passed as an http header through the environment rather than on
	// the command line, so it does not show up in the process list.
	if h := authHeader(auth); h != "" {
		cmd.Env = append(cmd.Env,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.extraHeader",
			"GIT_CONFIG_VALUE_0=Authorization: "+h,
		)
	}

	out, err := cmd.Output()
	if err != nil {
		var stderr string
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		if stderr != "" {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), stderr)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// authHeader returns the Authorization header value for the credentials, or the
// empty string when there are none. A token is a bearer; a username and password
// are basic.
func authHeader(auth Auth) string {
	switch {
	case auth.Token != "":
		return "Bearer " + auth.Token
	case auth.Username != "" || auth.Password != "":
		enc := base64.StdEncoding.EncodeToString([]byte(auth.Username + ":" + auth.Password))
		return "Basic " + enc
	default:
		return ""
	}
}

func isCommitSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}
