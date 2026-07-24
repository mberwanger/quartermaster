package target

import "strings"

// Markers delimit a managed block. They are HTML comments, so they are
// invisible wherever the markdown is rendered.
func markers(marker string) (begin, end string) {
	return "<!-- BEGIN " + marker + " -->", "<!-- END " + marker + " -->"
}

// ApplyBlock splices body between the markers in existing, returning the file's
// new content. An existing marked region is replaced, content outside it is
// preserved, and a file with no region yet gets the block appended. A file that
// is empty or absent becomes just the block.
func ApplyBlock(existing, marker, body string) string {
	begin, end := markers(marker)

	block := begin + "\n" + body
	if !strings.HasSuffix(block, "\n") {
		block += "\n"
	}
	block += end + "\n"

	if i := strings.Index(existing, begin); i >= 0 {
		if j := strings.Index(existing[i:], end); j >= 0 {
			tail := i + j + len(end)
			if k := strings.IndexByte(existing[tail:], '\n'); k >= 0 {
				tail += k + 1
			} else {
				tail = len(existing)
			}
			return existing[:i] + block + existing[tail:]
		}
	}

	if strings.TrimSpace(existing) == "" {
		return block
	}
	return strings.TrimRight(existing, "\n") + "\n\n" + block
}

// BlockRegion returns the body between the markers in existing, trimmed of the
// surrounding newlines, and whether a complete region was found. It is what
// verify compares against a freshly rendered block.
func BlockRegion(existing, marker string) (string, bool) {
	begin, end := markers(marker)

	_, rest, ok := strings.Cut(existing, begin)
	if !ok {
		return "", false
	}
	inner, _, ok := strings.Cut(rest, end)
	if !ok {
		return "", false
	}
	return strings.Trim(inner, "\n"), true
}
