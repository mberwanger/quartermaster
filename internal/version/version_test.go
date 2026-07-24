package version

import (
	"runtime/debug"
	"testing"
	"time"
)

const art = `  __ _ _ __ ___
 / _` + "`" + ` | '_ ` + "`" + ` _ \
| (_| | | | | | |
 \__, |_| |_| |_|
    |_|
`

func TestVersionText(t *testing.T) {
	sut := GetVersion(
		WithAsciiArt(art),
		WithBuiltBy("test"),
	)
	t.Log("\n" + sut.String())
	if sut.String() == "" {
		t.Fatal("should not be empty")
	}
}

func TestVersionJSON(t *testing.T) {
	sut := GetVersion()
	json, err := sut.JSONString()
	if err != nil {
		t.Fatal("expected no error, got", err)
	}

	if json == "" {
		t.Fatal("should not be empty")
	}
	t.Log("\n" + json)
}

func TestGetGitVersion(t *testing.T) {
	t.Run("null buildinfo", func(t *testing.T) {
		if got := getGitVersion(nil); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})
	t.Run("devel", func(t *testing.T) {
		if got := getGitVersion(&debug.BuildInfo{
			Main: debug.Module{
				Version: "(devel)",
			},
		}); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if got := getGitVersion(&debug.BuildInfo{}); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})
	t.Run("versioned", func(t *testing.T) {
		v := "1.0.0"
		if got := getGitVersion(&debug.BuildInfo{
			Main: debug.Module{
				Version: v,
			},
		}); got != v {
			t.Fatalf("expected %q, got %q", v, got)
		}
	})
}

func TestGetBuildDate(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := getBuildDate(&debug.BuildInfo{}); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		if got := getBuildDate(&debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.time", Value: "not a date"},
			},
		}); got != "" {
			t.Fatalf("expected an empty string, got %q", got)
		}
	})
	t.Run("time", func(t *testing.T) {
		now := time.Now()
		if got := getBuildDate(&debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.time", Value: now.Format("2006-01-02T15:04:05Z")},
			},
		}); got != now.Format("2006-01-02T15:04:05") {
			t.Fatalf("expected %q, got %q", now, got)
		}
	})
}

func TestGetKey(t *testing.T) {
	t.Run("nil buildinfo", func(t *testing.T) {
		if got := getKey(nil, "any"); got != "" {
			t.Fatalf("expected an empty string, got %q", got)
		}
	})
	t.Run("valid", func(t *testing.T) {
		key := "key"
		expect := "value"
		if got := getKey(&debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: key, Value: expect},
			},
		}, key); got != expect {
			t.Fatalf("expected %q, got %q", expect, got)
		}
	})
}
