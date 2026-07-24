package output

import (
	"fmt"
	"io"
)

// Writef writes a formatted string to w, ignoring write errors. It exists so
// callers on human-facing output paths (help text, error banners) do not have
// to thread an unrecoverable io error back up the stack.
func Writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
