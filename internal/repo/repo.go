// Package repo makes a consuming repository ready for Quartermaster: its
// gitignore entries and the git hooks that keep materialized knowledge current.
//
// Materialized output is gitignored, so git does not update it when a branch is
// switched or pulled. A branch that pins a different digest or changes the
// ruleset list leaves the rules stale with no visible signal. The hooks close
// that gap by running a sync after checkout and after merge.
package repo

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// gitignoreHeader marks the block Quartermaster manages in .gitignore.
const gitignoreHeader = "# Quartermaster (generated; safe to delete and regenerate)"

// EnsureGitignore adds any missing patterns to dir/.gitignore, idempotently, and
// returns the patterns it added. Patterns already present are left alone, so it
// is safe to run on every init.
func EnsureGitignore(dir string, patterns []string) ([]string, error) {
	path := filepath.Join(dir, ".gitignore")

	existing, err := os.ReadFile(path) //nolint:gosec // dir is a CLI flag
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	present := make(map[string]bool)
	sc := bufio.NewScanner(bytes.NewReader(existing))
	for sc.Scan() {
		present[strings.TrimSpace(sc.Text())] = true
	}

	var missing []string
	for _, p := range patterns {
		if !present[p] {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}

	var b bytes.Buffer
	b.Write(existing)
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
		b.WriteByte('\n')
	}
	if len(existing) > 0 {
		b.WriteByte('\n')
	}
	if !present[gitignoreHeader] {
		fmt.Fprintf(&b, "%s\n", gitignoreHeader)
	}
	for _, p := range missing {
		fmt.Fprintf(&b, "%s\n", p)
	}

	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		return nil, err
	}
	return missing, nil
}

// hookMarker identifies a hook Quartermaster wrote, so a later init updates its
// own hook but never clobbers one the user wrote.
const hookMarker = "quartermaster-managed"

// managedHooks are the git hooks that keep materialized knowledge current.
var managedHooks = []string{"post-checkout", "post-merge"}

func hookScript() string {
	return "#!/bin/sh\n" +
		"# " + hookMarker + ": keep Quartermaster's materialized knowledge current.\n" +
		"command -v qm >/dev/null 2>&1 && qm sync --quiet\n"
}

// InstallHooks writes the managed git hooks into the repository at dir. It
// returns the hooks it installed and the ones it skipped because a hook already
// existed that Quartermaster did not write. A directory that is not a git
// repository installs nothing and is not an error.
func InstallHooks(dir string) (installed, skipped []string, err error) {
	hooksDir, ok, err := hooksDir(dir)
	if err != nil || !ok {
		return nil, nil, err
	}
	if err := os.MkdirAll(hooksDir, 0o750); err != nil {
		return nil, nil, err
	}

	script := []byte(hookScript())
	for _, name := range managedHooks {
		p := filepath.Join(hooksDir, name)
		existing, err := os.ReadFile(p) //nolint:gosec // path derived from the repo's own .git
		switch {
		case err == nil && !bytes.Contains(existing, []byte(hookMarker)):
			skipped = append(skipped, name)
			continue
		case err != nil && !os.IsNotExist(err):
			return installed, skipped, err
		}
		if err := os.WriteFile(p, script, 0o750); err != nil { //nolint:gosec // hooks must be executable
			return installed, skipped, err
		}
		installed = append(installed, name)
	}
	return installed, skipped, nil
}

// hooksDir resolves the hooks directory git will actually run for dir.
//
// Hooks are per repository rather than per worktree: git resolves them against
// the shared git directory, so a hook written into a linked worktree's own git
// directory is never run and the worktree silently stops syncing. Installing
// into the shared directory is also why this only has to happen once per
// repository however many worktrees it has.
func hooksDir(dir string) (string, bool, error) {
	common, ok, err := commonDir(dir)
	if err != nil || !ok {
		return "", false, err
	}
	return filepath.Join(common, "hooks"), true, nil
}
