package target

import "bytes"

// RuleBody is the text of a rule below its frontmatter: the document's prose,
// verbatim, with leading blank lines trimmed and a single trailing newline.
//
// Every harness that renders a rule, and the library that renders the same
// document into an instruction string, calls this. That is what makes a rule
// delivered one way the same text as the same rule delivered another, rather
// than several near-copies that drift.
func RuleBody(prose []byte) []byte {
	trimmed := bytes.TrimLeft(prose, "\n")
	if len(trimmed) == 0 || bytes.HasSuffix(trimmed, []byte("\n")) {
		return trimmed
	}
	return append(trimmed, '\n')
}
