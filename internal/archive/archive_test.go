package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRoundTrip(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "meta.json", `{"format":"0.3"}`)
	writeFile(t, src, "store/engineering/a.md", "# A\n")
	writeFile(t, src, "store/index.md", "# root\n")

	data, err := TarGz(src)
	if err != nil {
		t.Fatalf("targz: %v", err)
	}

	dst := t.TempDir()
	if err := UntarGz(data, dst); err != nil {
		t.Fatalf("untargz: %v", err)
	}

	for _, rel := range []string{"meta.json", "store/engineering/a.md", "store/index.md"} {
		got, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
		want, _ := os.ReadFile(filepath.Join(src, filepath.FromSlash(rel)))
		if !bytes.Equal(got, want) {
			t.Fatalf("%s differs: got %q want %q", rel, got, want)
		}
	}
}

// TestDeterministic is the property that keeps republishing an unchanged bundle
// from churning the registry with a new layer digest.
func TestDeterministic(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "b.md", "b\n")
	writeFile(t, src, "a.md", "a\n")

	first, err := TarGz(src)
	if err != nil {
		t.Fatal(err)
	}

	// Touching the files changes their mtimes; the archive must not notice.
	stamp := time.Unix(1, 0)
	if err := os.Chtimes(filepath.Join(src, "a.md"), stamp, stamp); err != nil {
		t.Fatal(err)
	}

	second, err := TarGz(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("archive bytes are not deterministic")
	}
}

func TestUntarRejectsTraversal(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	body := []byte("pwned")
	if err := tw.WriteHeader(&tar.Header{
		Name: "../escaped.txt", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = zw.Close()

	if err := UntarGz(buf.Bytes(), t.TempDir()); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

func TestUntarRejectsAbsolute(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{
		Name: "/etc/pwned", Mode: 0o644, Size: 0, Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = zw.Close()

	if err := UntarGz(buf.Bytes(), t.TempDir()); err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
}
