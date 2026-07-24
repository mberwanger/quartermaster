package repo

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestEnsureGitignore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := EnsureGitignore(dir, []string{"/.quartermaster/", "/.claude/rules/qm/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 2 {
		t.Fatalf("added %v, want 2", added)
	}

	body, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	s := string(body)
	if !strings.Contains(s, "node_modules/") {
		t.Fatal("existing entry lost")
	}
	for _, want := range []string{gitignoreHeader, "/.quartermaster/", "/.claude/rules/qm/"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q", want)
		}
	}

	// Idempotent: a second run adds nothing.
	added, err = EnsureGitignore(dir, []string{"/.quartermaster/", "/.claude/rules/qm/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 0 {
		t.Fatalf("second run added %v, want none", added)
	}
	if strings.Count(string(mustRead(t, dir)), "/.quartermaster/") != 1 {
		t.Fatal("entry duplicated on second run")
	}
}

func TestEnsureGitignoreNoFile(t *testing.T) {
	dir := t.TempDir()
	added, err := EnsureGitignore(dir, []string{"/.quartermaster/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 {
		t.Fatalf("added %v, want 1", added)
	}
}

func TestInstallHooks(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	installed, skipped, err := InstallHooks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 2 || len(skipped) != 0 {
		t.Fatalf("installed=%v skipped=%v, want 2/0", installed, skipped)
	}

	hook := filepath.Join(dir, ".git", "hooks", "post-merge")
	fi, err := os.Stat(hook)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o100 == 0 {
		t.Fatal("hook is not executable")
	}

	// A managed hook is updated in place, not skipped.
	installed, skipped, _ = InstallHooks(dir)
	if len(installed) != 2 || len(skipped) != 0 {
		t.Fatalf("re-run installed=%v skipped=%v, want 2/0", installed, skipped)
	}

	// A hook the user wrote is never clobbered.
	if err := os.WriteFile(filepath.Join(dir, ".git", "hooks", "post-checkout"), []byte("#!/bin/sh\necho mine\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	installed, skipped, _ = InstallHooks(dir)
	if !slices.Contains(skipped, "post-checkout") {
		t.Fatalf("expected post-checkout skipped, got installed=%v skipped=%v", installed, skipped)
	}
}

// Git resolves hooks against the shared git directory, so installing into a
// linked worktree's own git directory would write a hook git never runs.
func TestInstallHooksFromWorktreeLandsInTheSharedDir(t *testing.T) {
	main, linked := worktreePair(t, "")

	installed, skipped, err := InstallHooks(linked)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 2 || len(skipped) != 0 {
		t.Fatalf("installed=%v skipped=%v, want 2/0", installed, skipped)
	}

	for _, name := range managedHooks {
		if _, err := os.Stat(filepath.Join(main, ".git", "hooks", name)); err != nil {
			t.Errorf("%s not installed in the shared git directory: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(main, ".git", "worktrees", "feature", "hooks", name)); err == nil {
			t.Errorf("%s installed where git will not look for it", name)
		}
	}
}

func TestInstallHooksNotAGitRepo(t *testing.T) {
	installed, skipped, err := InstallHooks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 0 || len(skipped) != 0 {
		t.Fatalf("non-repo installed=%v skipped=%v, want none", installed, skipped)
	}
}

func mustRead(t *testing.T, dir string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
