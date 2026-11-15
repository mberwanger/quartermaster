package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mberwanger/quartermaster/internal/manifest"
	"github.com/mberwanger/quartermaster/internal/plan"
	"github.com/mberwanger/quartermaster/internal/state"
)

// sessionSync is the sync an agent session runs when it opens. What it prints
// is not for a person: the harness adds a session hook's output to the
// agent's context, so this is how an agent in a fresh clone, with no memory and
// no rule read yet, learns that the knowledge exists, where, and how to search
// it.
//
// It never fails. A session hook that exits non-zero shows a person an error
// at the start of every session, and the common cause, no credentials for the
// registry, is not something the session can fix by itself. The tree already on
// disk is still the best answer there is, so the note says how old it is and
// what went wrong, and the agent can tell the person.
func sessionSync(dir string, out io.Writer) {
	if _, err := os.Stat(filepath.Join(dir, manifest.FileName)); err != nil {
		return
	}

	err := syncRepo(dir, io.Discard, true)
	n := treeSize(dir)

	var b strings.Builder
	b.WriteString("Quartermaster knowledge for this repository:\n")
	switch {
	case err == nil:
		fmt.Fprintf(&b, "- %d documents in %s/, refreshed from the store just now.\n", n, plan.KnowledgeDir)
	case n > 0:
		fmt.Fprintf(&b, "- %d documents in %s/, last synced %s. Refreshing failed: %s\n", n, plan.KnowledgeDir, lastSynced(dir), reason(err))
		b.WriteString("  Use the tree as it is. Tell the person, and once they say access is fixed, run `qm sync`.\n")
	default:
		fmt.Fprintf(&b, "- This repository declares a knowledge store in %s, but none has been delivered: %s\n", manifest.FileName, reason(err))
		b.WriteString("  Tell the person. Do not substitute another checkout of the store; it is not what this repository is given.\n")
		_, _ = io.WriteString(out, b.String())
		return
	}
	b.WriteString("- Search it first, before any grep: `qm find <words>` (a question, a phrase or a heading) lists the documents with the lines that matched. `qm show <id or /link.md>` prints one, following what superseded it.\n")
	fmt.Fprintf(&b, "- The tree is gitignored, so search tools that honour .gitignore skip it. Grep it only by naming %s/, and never search another checkout of the store.\n", plan.KnowledgeDir)
	_, _ = io.WriteString(out, b.String())
}

// treeSize counts the documents on disk that a search can return, whatever
// the sync just did. A file with no id, such as a skill's reference, is not
// one, which keeps the count the same as the guide's.
func treeSize(dir string) int {
	entries, err := loadTree(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.ID() != "" {
			n++
		}
	}
	return n
}

// lastSynced is when the state file was last written, which is when a sync
// last finished.
func lastSynced(dir string) string {
	fi, err := os.Stat(filepath.Join(dir, state.Dir, "state.json"))
	if err != nil {
		return "at an unknown time"
	}
	return fi.ModTime().Format(time.DateOnly)
}

// reason shortens a sync error to what an agent can act on. A registry's error
// nests the request three times, and the useful part is that it refused.
func reason(err error) string {
	msg := err.Error()
	low := strings.ToLower(msg)
	for _, s := range []string{"status code 401", "status code 403", "unauthenticated", "unauthorized"} {
		if strings.Contains(low, s) {
			return "the registry refused the request, so this machine is not signed in to it or has no access."
		}
	}
	var nf interface{ Timeout() bool }
	if errors.As(err, &nf) && nf.Timeout() {
		return "the store did not answer in time."
	}
	for _, s := range []string{"no such host", "connection refused", "network is unreachable", "dial tcp"} {
		if strings.Contains(low, s) {
			return "the store could not be reached from this machine."
		}
	}
	if r := []rune(msg); len(r) > 200 {
		msg = string(r[:199]) + "…"
	}
	return msg
}
