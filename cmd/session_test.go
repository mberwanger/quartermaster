package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sessionRepo(t *testing.T, source string) string {
	t.Helper()
	dir := t.TempDir()
	body := "bundles:\n  - source: " + source + "\n    use: [engineering]\ntargets:\n  - claude\ntelemetry: true\n"
	if err := os.WriteFile(filepath.Join(dir, ".qm.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A session that opens on a repository whose store answers is told where the
// tree is and how to search it.
func TestSessionSyncTellsTheAgentWhereTheTreeIs(t *testing.T) {
	store, err := filepath.Abs(exampleStore)
	if err != nil {
		t.Fatal(err)
	}
	dir := sessionRepo(t, "file://"+store)

	var out bytes.Buffer
	sessionSync(dir, &out)
	for _, want := range []string{"documents in .qm/knowledge/, refreshed from the store just now", "qm find", "qm show", "gitignored", "never search another checkout"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("note lacks %q:\n%s", want, out.String())
		}
	}
}

// A store that cannot be reached leaves the tree on disk in use, and the note
// says so, rather than failing the session.
func TestSessionSyncWhenTheStoreFails(t *testing.T) {
	store, err := filepath.Abs(exampleStore)
	if err != nil {
		t.Fatal(err)
	}
	dir := sessionRepo(t, "file://"+store)
	if err := syncRepo(dir, &bytes.Buffer{}, true); err != nil {
		t.Fatal(err)
	}
	gone := sessionRepo(t, "file://"+filepath.Join(t.TempDir(), "gone"))
	if err := os.WriteFile(filepath.Join(dir, ".qm.yaml"), mustRead(t, filepath.Join(gone, ".qm.yaml")), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	sessionSync(dir, &out)
	if !strings.Contains(out.String(), "Refreshing failed") || !strings.Contains(out.String(), "Use the tree as it is") {
		t.Errorf("note does not say the refresh failed:\n%s", out.String())
	}

	// A fresh clone has no tree at all, and the note says not to look
	// elsewhere for one.
	out.Reset()
	sessionSync(gone, &out)
	if !strings.Contains(out.String(), "none has been delivered") || !strings.Contains(out.String(), "Do not substitute another checkout") {
		t.Errorf("fresh-clone note:\n%s", out.String())
	}
}

// A directory with no manifest is not a consuming repository, and the hook
// says nothing there.
func TestSessionSyncIsSilentWithoutAManifest(t *testing.T) {
	var out bytes.Buffer
	sessionSync(t.TempDir(), &out)
	if out.Len() != 0 {
		t.Errorf("printed without a manifest:\n%s", out.String())
	}
}

func TestReasonNamesARefusal(t *testing.T) {
	got := reason(errors.New(`HEAD "https://r/manifests/latest": response status code 403: denied: Unauthenticated request`))
	if !strings.Contains(got, "refused") {
		t.Errorf("reason = %q", got)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A sync a person runs offers the session hook and writes it only when asked,
// and a quiet sync, which hooks run, never touches the committed settings.
func TestSyncAndTheSessionHook(t *testing.T) {
	store, err := filepath.Abs(exampleStore)
	if err != nil {
		t.Fatal(err)
	}
	dir := sessionRepo(t, "file://"+store)
	settings := filepath.Join(dir, ".claude", "settings.json")

	if err := syncRepo(dir, &bytes.Buffer{}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(settings); err == nil {
		t.Error("a quiet sync wrote the settings")
	}

	var out bytes.Buffer
	if err := syncRepo(dir, &out, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "run qm sync --install-hook") {
		t.Errorf("sync did not offer the hook:\n%s", out.String())
	}
	if _, err := os.Stat(settings); err == nil {
		t.Error("sync installed the hook without being asked")
	}

	out.Reset()
	if err := syncRepoWith(dir, &out, syncOptions{installHook: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "wrote the session hook") || !strings.Contains(string(mustRead(t, settings)), "qm sync --session") {
		t.Errorf("--install-hook did not write it:\n%s", out.String())
	}

	// A settings file whose hooks are not the shape the harness reads is
	// reported, and the knowledge still syncs.
	if err := os.WriteFile(settings, []byte(`{"hooks":{"SessionStart":"qm sync --session"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := syncRepo(dir, &out, false); err != nil {
		t.Fatalf("a broken settings file stopped the sync: %v", err)
	}
	if !strings.Contains(out.String(), "left the session hook alone") {
		t.Errorf("no warning:\n%s", out.String())
	}
}

// A local permission error is not reported as the registry refusing.
func TestReasonKeepsALocalError(t *testing.T) {
	if got := reason(errors.New("open .qm/state.json: permission denied")); strings.Contains(got, "registry") {
		t.Errorf("reason = %q", got)
	}
}
