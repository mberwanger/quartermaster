package provider

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newGitRepo builds a small repository on disk and returns its path. A local
// path is a valid git remote, so the fetch machinery can be exercised without a
// network or a server.
//
// The test is isolated from the developer's own git configuration. A global
// commit.gpgsign, a hooks path, or a template directory would otherwise reach
// into these commands, and a signing key that needs an agent turns a unit test
// into something that fails when the agent is busy.
func newGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "store"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "store", "bundle.yaml"), []byte("name: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"init", "--quiet", "-b", "main"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
		{"config", "commit.gpgsign", "false"},
		{"add", "."},
		{"commit", "--quiet", "--no-gpg-sign", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

func TestGitResolveAndFetch(t *testing.T) {
	origin := newGitRepo(t)

	sha, err := gitResolve(origin, "main", Auth{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(sha) != 40 {
		t.Fatalf("sha = %q, want a 40-char commit", sha)
	}

	// HEAD resolves to the same commit.
	head, err := gitResolve(origin, "", Auth{})
	if err != nil {
		t.Fatalf("resolve HEAD: %v", err)
	}
	if head != sha {
		t.Fatalf("HEAD = %s, main = %s", head, sha)
	}

	dst := t.TempDir()
	if err := gitFetch(origin, "main", dst, Auth{}); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "store", "bundle.yaml")); err != nil {
		t.Fatalf("tree not fetched: %v", err)
	}
	// The history is not part of the bundle.
	if _, err := os.Stat(filepath.Join(dst, ".git")); !os.IsNotExist(err) {
		t.Fatal(".git survived the fetch")
	}
}

func TestGitResolveUnknownRef(t *testing.T) {
	origin := newGitRepo(t)
	if _, err := gitResolve(origin, "no-such-ref", Auth{}); err == nil {
		t.Fatal("expected an error for an unknown ref")
	}
}
