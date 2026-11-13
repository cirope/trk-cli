package cli

import (
	"errors"
	"fmt"
	"io"
)

// Exit codes, as documented in SPEC 4.8. They are a public contract: an
// agent branches on them.
const (
	ExitOK       = 0 // Done.
	ExitError    = 1 // Something failed (network, server error, unexpected).
	ExitUsage    = 2 // Wrong usage (unknown flag, missing argument).
	ExitNotFound = 3 // Not found, or not yours.
	ExitAuth     = 4 // Not signed in, or the session expired or was revoked.
	ExitRefused  = 5 // Refused: understood and not allowed.
)

// usageError marks an error caused by how the command was invoked.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// exitCode picks the exit code for err.
func exitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ue usageError
	if errors.As(err, &ue) {
		return ExitUsage
	}
	return ExitError
}

// reportError writes err as one sentence to w, with a hint for usage
// errors.
func reportError(w io.Writer, err error) {
	fmt.Fprintf(w, "trk: %s\n", err)
	var ue usageError
	if errors.As(err, &ue) {
		fmt.Fprintln(w, "Run 'trk --help' for usage.")
	}
}
