package cmd

// exitError wraps an error with an explicit process exit code. Commands return
// it when a specific non-zero code is meaningful to callers and scripts.
type exitError struct {
	err  error
	code int
}

func (e *exitError) Error() string {
	return e.err.Error()
}

func (e *exitError) Unwrap() error {
	return e.err
}

// formatError returns a user-friendly message for an error surfaced to the
// terminal. Quartermaster makes no network calls, so there is no wire framing
// to strip; the error's own message is the message.
func formatError(err error) string {
	return err.Error()
}
