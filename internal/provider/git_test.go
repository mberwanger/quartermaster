package provider

import "testing"

func TestParseGitSource(t *testing.T) {
	cases := []struct {
		in               string
		url, subdir, ref string
	}{
		{
			in:  "https://github.com/org/repo.git",
			url: "https://github.com/org/repo.git",
		},
		{
			in:  "https://github.com/org/repo.git#v1.2.3",
			url: "https://github.com/org/repo.git", ref: "v1.2.3",
		},
		{
			in:  "https://github.com/org/repo.git//store",
			url: "https://github.com/org/repo.git", subdir: "store",
		},
		{
			in:  "https://github.com/org/repo.git//store#main",
			url: "https://github.com/org/repo.git", subdir: "store", ref: "main",
		},
		{
			in:  "https://github.com/org/repo.git//nested/store#abc123",
			url: "https://github.com/org/repo.git", subdir: "nested/store", ref: "abc123",
		},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			url, subdir, ref := parseGitSource(tc.in)
			if url != tc.url || subdir != tc.subdir || ref != tc.ref {
				t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)",
					url, subdir, ref, tc.url, tc.subdir, tc.ref)
			}
		})
	}
}
