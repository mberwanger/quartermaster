package provider

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestAuthHeader(t *testing.T) {
	if h := authHeader(Auth{}); h != "" {
		t.Fatalf("no credentials should yield no header, got %q", h)
	}

	if h := authHeader(Auth{Token: "abc"}); h != "Bearer abc" {
		t.Fatalf("token header = %q", h)
	}

	h := authHeader(Auth{Username: "u", Password: "p"})
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("u:p"))
	if h != want {
		t.Fatalf("basic header = %q, want %q", h, want)
	}

	// A token wins over basic when both are somehow set, matching the header
	// precedence every scheme uses.
	if h := authHeader(Auth{Token: "abc", Username: "u"}); !strings.HasPrefix(h, "Bearer ") {
		t.Fatalf("token should win, got %q", h)
	}
}

func TestAuthSet(t *testing.T) {
	if (Auth{}).set() {
		t.Fatal("zero Auth should be unset")
	}
	for _, a := range []Auth{{Token: "t"}, {Username: "u"}, {Password: "p"}} {
		if !a.set() {
			t.Fatalf("%+v should be set", a)
		}
	}
}
