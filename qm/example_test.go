package qm_test

import (
	"fmt"

	"github.com/mberwanger/quartermaster/qm"
)

// This is the whole integration an agent needs: resolve a bundle, render the
// selected rulesets into the agent's instructions, and expose the catalog and a
// fetch as its two tools. No files are written, and no agent framework is
// imported.
func Example() {
	bundle, err := qm.Open("oci://ghcr.io/org/knowledge:v1")
	if err != nil {
		// handle it
		return
	}

	// The agent's own instructions.
	instructions, err := bundle.Instruction("engineering", "billing")
	if err != nil {
		return
	}
	_ = instructions

	// Tool one: list what exists, by id and description.
	for _, e := range bundle.Catalog() {
		fmt.Printf("%s: %s\n", e.ID, e.Description)
	}

	// Tool two: fetch the one the agent chose.
	doc, err := bundle.Document("eng.error-handling")
	if err != nil {
		return
	}
	_ = doc
}
