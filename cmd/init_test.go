package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A manifest follows its source's tag unless the author asked to pin, because
// the hooks keep an unpinned repository current and a pin is a reason to drift.
func TestWriteManifestPinsOnlyWhenAsked(t *testing.T) {
	for name, tc := range map[string]struct {
		pin  bool
		want bool
	}{
		"unpinned by default": {pin: false, want: false},
		"pinned on request":   {pin: true, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), ".qm.yaml")
			err := writeManifest(p, []bundleSpec{{
				source:   "oci://ghcr.io/org/knowledge:latest",
				digest:   "sha256:abc",
				pin:      tc.pin,
				packages: []string{"go-service"},
			}}, []string{"claude"})
			if err != nil {
				t.Fatal(err)
			}
			body, _ := os.ReadFile(p)
			if got := strings.Contains(string(body), "digest:"); got != tc.want {
				t.Errorf("digest written = %v, want %v:\n%s", got, tc.want, body)
			}
			if strings.Contains(string(body), "telemetry") {
				t.Errorf("manifest still declares telemetry:\n%s", body)
			}
		})
	}
}
