package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cirope/trk-cli/internal/api"
)

// Exit codes, as documented in --help. They are a public contract: an
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

// refusedError marks something trk understood and will not do.
type refusedError struct{ err error }

func (e refusedError) Error() string { return e.err.Error() }
func (e refusedError) Unwrap() error { return e.err }

// silentError has been reported already; it only carries the exit code.
type silentError struct{ code int }

func (e silentError) Error() string { return fmt.Sprintf("exit %d", e.code) }

// classify picks the exit code and the JSON error code for err.
func classify(err error) (int, string) {
	var (
		ue usageError
		re refusedError
		ns notSignedIn
		se silentError
	)
	switch {
	case err == nil:
		return ExitOK, ""
	case errors.As(err, &se):
		return se.code, ""
	case errors.As(err, &ue), errors.Is(err, api.ErrBadID):
		return ExitUsage, "usage"
	case errors.As(err, &ns), errors.Is(err, api.ErrUnauthorized):
		return ExitAuth, "unauthorized"
	case errors.Is(err, api.ErrNotFound):
		return ExitNotFound, "not_found"
	case errors.As(err, &re), errors.Is(err, api.ErrForbidden), errors.Is(err, api.ErrConflict):
		return ExitRefused, "refused"
	}
	return ExitError, "error"
}

func exitCode(err error) int {
	code, _ := classify(err)
	return code
}

// message is err as one sentence for a person.
func message(err error) string {
	if errors.Is(err, api.ErrUnauthorized) {
		return "your session expired or was revoked; run trk login to sign in again"
	}
	return err.Error()
}

// reportError writes err to stderr: one line, or a JSON object under
// --json.
func (a *app) reportError(err error) {
	code, kind := classify(err)
	if kind == "" {
		return
	}
	w := a.env.Stderr
	if a.flags.json {
		b, _ := json.Marshal(map[string]string{"code": kind, "message": message(err)})
		fmt.Fprintf(w, "%s\n", b)
		return
	}
	fmt.Fprintf(w, "trk: %s\n", message(err))
	if code == ExitUsage {
		fmt.Fprintln(w, "Run 'trk --help' for usage.")
	}
}
