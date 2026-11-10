package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

// A repository that has not synced since the rename still resolves, so status
// and verify work before the migration sync has run.
func TestLoadFallsBackToTheLegacyName(t *testing.T) {
	dir := t.TempDir()
	body := "bundles:\n  - source: file://store\n    use: [a]\ntargets: [claude]\ntelemetry: true\n"
	if err := os.WriteFile(filepath.Join(dir, LegacyFileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Bundles) != 1 || m.Bundles[0].Source != "file://store" {
		t.Errorf("legacy manifest not read: %+v", m)
	}
}

func TestLoadPrefersTheCurrentName(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		LegacyFileName: "targets: [cursor]\n",
		FileName:       "targets: [claude]\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Targets) != 1 || m.Targets[0] != "claude" {
		t.Errorf("read the legacy file over the current one: %+v", m)
	}
}
