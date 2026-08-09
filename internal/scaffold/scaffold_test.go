package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/index"
	"github.com/mberwanger/quartermaster/internal/validate"
)

func TestWriteSubstitutesName(t *testing.T) {
	dir := t.TempDir()
	written, err := Write(dir, `my: "store"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) == 0 {
		t.Fatal("nothing written")
	}

	storeConfig, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if storeConfig.Name != `my: "store"` {
		t.Fatalf("name = %q", storeConfig.Name)
	}

	yaml, err := os.ReadFile(filepath.Join(dir, ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(yaml), "{{") {
		t.Fatalf("an unsubstituted token survived:\n%s", yaml)
	}
}

// Every embedded file goes through the same substitution pass, so a token
// left in any one of them, not just bundle.yaml, means Write silently shipped
// a template placeholder into a real store.
func TestWriteLeavesNoTokenInAnyFile(t *testing.T) {
	dir := t.TempDir()
	written, err := Write(dir, "my-store")
	if err != nil {
		t.Fatal(err)
	}

	for _, rel := range written {
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "{{") {
			t.Fatalf("%s: an unsubstituted token survived:\n%s", rel, body)
		}
	}
}

func TestWriteRejectsInvalidNameBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "bad\nname"); err == nil {
		t.Fatal("multiline name was accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, ConfigFile)); !os.IsNotExist(err) {
		t.Fatalf("bundle.yaml was written before rejecting the name: %v", err)
	}
}

func TestWriteDeclaresCurrentOKFVersion(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "my-store"); err != nil {
		t.Fatal(err)
	}

	rootIndex, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nokf_version: \"" + index.CurrentOKFVersion + "\"\n---\n"
	if !strings.HasPrefix(string(rootIndex), want) {
		t.Fatalf("root index does not declare the current OKF version %q:\n%s", index.CurrentOKFVersion, rootIndex)
	}
}

// The whole promise of init is that what it writes is already sound. If the
// scaffold and the validator ever disagree, this fails.
func TestScaffoldValidates(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "my-store"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := validate.Run(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("scaffold did not validate: %+v", res.Findings)
	}
	if res.Checked == 0 {
		t.Fatal("expected the scaffold to include at least the template document")
	}
}

// The scaffold ships two files that have to agree and are edited separately: the
// schema says which status values a document may declare, and bundle.yaml says
// which of them may become a rule. Nothing else notices when they drift, because
// a store whose gate names a status the schema forbids still validates: every
// document is well-formed, and none of them can ever be promoted. That is how
// the scaffold spent a release asking for "active" while its own schema had
// moved to draft, stable, and deprecated.
func TestScaffoldGateAdmitsAStatusTheSchemaAllows(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "my-store"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "meta", "frontmatter.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties struct {
			Status struct {
				Enum []string `json:"enum"`
			} `json:"status"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.Properties.Status.Enum) == 0 {
		t.Fatal("the scaffold schema constrains no status values, so the gate cannot be checked against it")
	}

	var admitted []string
	for _, status := range schema.Properties.Status.Enum {
		if ok, _ := cfg.Requires.Allows(map[string]any{"status": status}); ok {
			admitted = append(admitted, status)
		}
	}
	if len(admitted) == 0 {
		t.Fatalf("no status the schema allows (%v) passes the scaffold's requires gate; nothing could ever become a rule",
			schema.Properties.Status.Enum)
	}

	// A draft is distributed and readable, and never loads on its own. That is
	// the one promise the scaffold's comments make about the gate.
	if ok, _ := cfg.Requires.Allows(map[string]any{"status": "draft"}); ok {
		t.Error("the scaffold's gate promotes a draft, which its own template document is")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	if Exists(dir) {
		t.Fatal("an empty directory reported as a store")
	}
	if _, err := Write(dir, "x"); err != nil {
		t.Fatal(err)
	}
	if !Exists(dir) {
		t.Fatal("a scaffolded directory not reported as a store")
	}
}

// A store missing only bundle.yaml is still a store: Exists must not require
// that one file specifically, or a caller's --force gate silently overwrites
// every other scaffold-owned file.
func TestExistsDetectsAnyScaffoldOwnedFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "x"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, ConfigFile)); err != nil {
		t.Fatal(err)
	}

	if !Exists(dir) {
		t.Fatal("a store missing only bundle.yaml was not reported as existing")
	}
}

// A destination blocked by a directory must fail before any scaffold file is
// moved into place, so a doomed init never leaves a half-written store behind.
func TestWriteFailsAtomicallyOnDirectoryCollision(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "meta", "packages.yaml")
	if err := os.MkdirAll(blocked, 0o750); err != nil {
		t.Fatal(err)
	}

	if _, err := Write(dir, "x"); err == nil {
		t.Fatal("a directory collision was accepted")
	}

	if _, err := os.Stat(filepath.Join(dir, ConfigFile)); !os.IsNotExist(err) {
		t.Fatalf("bundle.yaml was written despite a later collision: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.md")); !os.IsNotExist(err) {
		t.Fatalf("index.md was written despite a later collision: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".qm-init-") {
			t.Fatalf("staging directory %q was not cleaned up", e.Name())
		}
	}
}

// A plain file sitting where the scaffold needs a directory (here "meta",
// which several scaffold-owned files nest under) is a different collision
// shape than the destination itself being a directory: MkdirAll fails on it
// partway through commitStaged rather than during the upfront Lstat check,
// unless every ancestor segment of every path is checked too.
func TestWriteFailsAtomicallyWhenAnAncestorIsAPlainFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "meta"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Write(dir, "x"); err == nil {
		t.Fatal("a file blocking a scaffold directory was accepted")
	}

	if _, err := os.Stat(filepath.Join(dir, ConfigFile)); !os.IsNotExist(err) {
		t.Fatalf("bundle.yaml was written despite a later collision: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.md")); !os.IsNotExist(err) {
		t.Fatalf("index.md was written despite a later collision: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".qm-init-") {
			t.Fatalf("staging directory %q was not cleaned up", e.Name())
		}
	}
}
