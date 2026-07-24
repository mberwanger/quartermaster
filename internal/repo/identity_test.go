package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoteIdentity(t *testing.T) {
	// The point of the reduction: every spelling of one repository, and every
	// developer's clone of it, reduce to the same name.
	same := []string{
		"https://github.com/admiral/admiral-api.git",
		"https://github.com/admiral/admiral-api",
		"git@github.com:admiral/admiral-api.git",
		"ssh://git@github.com/admiral/admiral-api.git",
		"ssh://git@github.com:22/admiral/admiral-api",
		"https://someone:token@github.com/admiral/admiral-api.git",
		"https://github.com/Admiral/Admiral-API.git",
		"https://github.com/admiral/admiral-api/",
	}
	for _, url := range same {
		if got := remoteIdentity(url); got != "github.com/admiral/admiral-api" {
			t.Errorf("remoteIdentity(%q) = %q", url, got)
		}
	}

	other := map[string]string{
		"/srv/git/admiral-api.git":   "admiral-api",
		"https://github.com":         "",
		"":                           "",
		"git@gitlab.example.com:g/p": "gitlab.example.com/g/p",
	}
	for url, want := range other {
		if got := remoteIdentity(url); got != want {
			t.Errorf("remoteIdentity(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestRemoteURLPrefersOrigin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := `[core]
	bare = false
	url = not-a-remote
[remote "upstream"]
	url = https://github.com/admiral/upstream.git
[remote "origin"]
	url = https://github.com/admiral/admiral-api.git
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := remoteURL(path)
	if !ok || got != "https://github.com/admiral/admiral-api.git" {
		t.Fatalf("remoteURL = %q, %v", got, ok)
	}
}

// A repository whose remote is named something else is still identified, rather
// than falling back to a directory name that says less.
func TestRemoteURLFallsBackToFirstRemote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte("[remote \"fork\"]\n\turl = git@github.com:me/fork.git\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := remoteURL(path)
	if !ok || got != "git@github.com:me/fork.git" {
		t.Fatalf("remoteURL = %q, %v", got, ok)
	}
}

func TestRemoteURLNoRemote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte("[core]\n\tbare = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := remoteURL(path); ok {
		t.Fatal("a config with no remote should report none")
	}
}

// The case the whole file exists for: a main worktree and a linked worktree of
// the same repository must produce one identity, not two.
func TestIdentityIsSharedAcrossWorktrees(t *testing.T) {
	main, linked := worktreePair(t, "https://github.com/admiral/admiral-api.git")

	want := "github.com/admiral/admiral-api"
	if got := Identity(main); got != want {
		t.Errorf("main worktree identity = %q, want %q", got, want)
	}
	if got := Identity(linked); got != want {
		t.Errorf("linked worktree identity = %q, want %q", got, want)
	}
}

// Without a remote the shared git directory still names the repository, and it
// is still shared, so the worktrees continue to agree.
func TestIdentityWithoutRemote(t *testing.T) {
	main, linked := worktreePair(t, "")

	if got, want := Identity(main), "main"; got != want {
		t.Errorf("main worktree identity = %q, want %q", got, want)
	}
	if got, want := Identity(linked), "main"; got != want {
		t.Errorf("linked worktree identity = %q, want %q", got, want)
	}
}

// The branch is the one piece of git state a worktree does not share, which is
// most of the reason to have several, so it must come from the worktree's own
// git directory rather than the shared one.
func TestBranchIsPerWorktree(t *testing.T) {
	main, linked := worktreePair(t, "")

	if got, want := Branch(main), "master"; got != want {
		t.Errorf("main branch = %q, want %q", got, want)
	}
	if got, want := Branch(linked), "feature"; got != want {
		t.Errorf("linked branch = %q, want %q", got, want)
	}
}

func TestBranchDetachedOrAbsent(t *testing.T) {
	main, _ := worktreePair(t, "")
	if err := os.WriteFile(filepath.Join(main, ".git", "HEAD"), []byte("9fceb02b0c2f4b1ea0e3b8a4e5c6d7e8f9012345\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Branch(main); got != "" {
		t.Errorf("detached head reported branch %q, want none", got)
	}
	if got := Branch(t.TempDir()); got != "" {
		t.Errorf("non-repository reported branch %q, want none", got)
	}
}

// Work happens in subdirectories. Without walking up, a session that started in
// web/ belongs to a repository called "web", and one repository fragments into
// as many repositories as it has directories people work in.
func TestIdentityFromASubdirectory(t *testing.T) {
	main, linked := worktreePair(t, "https://github.com/admiral/admiral-app.git")

	for _, sub := range []string{"web", filepath.Join("server", "internal", "endpoint")} {
		dir := filepath.Join(main, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if got, want := Identity(dir), "github.com/admiral/admiral-app"; got != want {
			t.Errorf("Identity(%s) = %q, want %q", sub, got, want)
		}
	}

	// The same holds inside a linked worktree, where .git is a file.
	deep := filepath.Join(linked, "charts", "admiral-app")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := Identity(deep), "github.com/admiral/admiral-app"; got != want {
		t.Errorf("Identity(worktree subdir) = %q, want %q", got, want)
	}
}

// Without a remote, a subdirectory still resolves to the repository's own name
// rather than to its own.
func TestIdentityFromASubdirectoryWithoutRemote(t *testing.T) {
	main, _ := worktreePair(t, "")
	dir := filepath.Join(main, "web")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := Identity(dir), "main"; got != want {
		t.Errorf("Identity = %q, want %q", got, want)
	}
}

func TestIdentityOutsideAnyRepository(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "loose-checkout")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := Identity(dir), "loose-checkout"; got != want {
		t.Errorf("identity = %q, want %q", got, want)
	}
}

// worktreePair builds the on-disk shape git produces for a repository with one
// linked worktree, and returns both working trees. The layout is the contract
// being relied on: the linked worktree's .git is a file, its git directory holds
// a commondir pointing back at the shared one, and the config lives there.
func worktreePair(t *testing.T, remote string) (main, linked string) {
	t.Helper()

	root := t.TempDir()
	main = filepath.Join(root, "main")
	shared := filepath.Join(main, ".git")
	linkedGit := filepath.Join(shared, "worktrees", "feature")
	linked = filepath.Join(root, "feature")

	for _, d := range []string{shared, linkedGit, linked} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	config := "[core]\n\tbare = false\n"
	if remote != "" {
		config += "[remote \"origin\"]\n\turl = " + remote + "\n"
	}
	write := map[string]string{
		filepath.Join(shared, "config"):       config,
		filepath.Join(shared, "HEAD"):         "ref: refs/heads/master\n",
		filepath.Join(linkedGit, "commondir"): "../..\n",
		filepath.Join(linkedGit, "HEAD"):      "ref: refs/heads/feature\n",
		filepath.Join(linked, ".git"):         "gitdir: " + linkedGit + "\n",
	}
	for path, body := range write {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return main, linked
}
