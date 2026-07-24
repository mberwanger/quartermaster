package gate

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func parse(t *testing.T, src string) Gate {
	t.Helper()
	var g Gate
	if err := yaml.Unmarshal([]byte(src), &g); err != nil {
		t.Fatalf("unmarshal gate: %v", err)
	}
	return g
}

func TestGateAllows(t *testing.T) {
	g := parse(t, "status: [active]\nprovenance: [verified, decided]\nvisibility: { not: [restricted] }\n")

	cases := []struct {
		name string
		fm   map[string]any
		want bool
	}{
		{"passes", map[string]any{"status": "active", "provenance": "verified"}, true},
		{"passes with internal visibility", map[string]any{"status": "active", "provenance": "decided", "visibility": "internal"}, true},
		{"draft rejected", map[string]any{"status": "draft", "provenance": "verified"}, false},
		{"asserted rejected", map[string]any{"status": "active", "provenance": "asserted"}, false},
		{"restricted rejected", map[string]any{"status": "active", "provenance": "verified", "visibility": "restricted"}, false},
		{"missing status rejected by allowlist", map[string]any{"provenance": "verified"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := g.Allows(tc.fm)
			if got != tc.want {
				t.Fatalf("Allows = %v (reason %q), want %v", got, reason, tc.want)
			}
			if !got && reason == "" {
				t.Fatal("rejection must carry a reason")
			}
		})
	}
}

func TestEmptyGatePassesEverything(t *testing.T) {
	var g Gate
	if ok, _ := g.Allows(map[string]any{"status": "draft"}); !ok {
		t.Fatal("zero gate should pass everything")
	}
	if !g.Empty() {
		t.Fatal("zero gate should report Empty")
	}
}

// A list-valued field matches when any of its entries does, which is what makes
// filtering on tags useful.
func TestListValuedField(t *testing.T) {
	g := parse(t, "tags: [observability]\n")

	cases := []struct {
		name string
		tags any
		want bool
	}{
		{"one of several matches", []any{"go", "observability"}, true},
		{"sole entry matches", []any{"observability"}, true},
		{"no entry matches", []any{"go", "http"}, false},
		{"empty list matches nothing", []any{}, false},
		{"absent field matches nothing", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm := map[string]any{}
			if tc.tags != nil {
				fm["tags"] = tc.tags
			}
			got, reason := g.Allows(fm)
			if got != tc.want {
				t.Fatalf("Allows = %v (%s), want %v", got, reason, tc.want)
			}
		})
	}
}

// A denylist over a list field excludes a document when any entry is forbidden.
func TestListValuedDenylist(t *testing.T) {
	g := parse(t, "tags: { not: [internal-only] }\n")

	if ok, _ := g.Allows(map[string]any{"tags": []any{"go", "internal-only"}}); ok {
		t.Fatal("a forbidden tag should exclude the document")
	}
	if ok, _ := g.Allows(map[string]any{"tags": []any{"go"}}); !ok {
		t.Fatal("a document with no forbidden tag should pass")
	}
}

func TestBadPredicate(t *testing.T) {
	var g Gate
	if err := yaml.Unmarshal([]byte("status: \"active\"\n"), &g); err == nil {
		t.Fatal("expected error for scalar predicate")
	}
}
