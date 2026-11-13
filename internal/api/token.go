package api

import (
	"fmt"
	"log/slog"
)

const redacted = "[redacted]"

// Token is a session's bearer token. It redacts itself wherever it is
// printed, logged or encoded; only Reveal gives the value, for the
// Authorization header and the token store.
type Token string

// Reveal returns the secret value. Call it only to send or store it.
func (t Token) Reveal() string { return string(t) }

// String implements fmt.Stringer.
func (Token) String() string { return redacted }

// GoString implements fmt.GoStringer.
func (Token) GoString() string { return redacted }

// Format implements fmt.Formatter, so no verb (%x, %q...) shows the value.
func (Token) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(redacted)) }

// LogValue implements slog.LogValuer.
func (Token) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalJSON implements json.Marshaler.
func (Token) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }
