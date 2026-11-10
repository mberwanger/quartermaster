package target

import (
	"bytes"
	"fmt"
)

// sourceLine writes the provenance comment every generated file opens with. A
// document the tool wrote itself has no bundle to name, so that part is left
// out rather than printed empty.
func sourceLine(b *bytes.Buffer, id, digest, commit string) {
	fmt.Fprintf(b, "# source: %s", id)
	if digest != "" {
		fmt.Fprintf(b, "  bundle: %s", digest)
	}
	if commit != "" {
		fmt.Fprintf(b, "  commit: %s", commit)
	}
	b.WriteByte('\n')
}

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

// withGuide is the rule set a harness renders: the guide first, when there is
// one, then the selected documents. The guide goes first because it is the
// document that says where the rest came from.
func withGuide(in Input) []Doc {
	if in.Guide == nil {
		return in.Docs
	}
	return append([]Doc{*in.Guide}, in.Docs...)
}
