package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const secret = "tok_s3cr3t_value_never_shown"

func TestTokenNeverPrints(t *testing.T) {
	tok := Token(secret)
	type wrapper struct{ T Token }

	outputs := map[string]string{
		"%s":  fmt.Sprintf("%s", tok),
		"%v":  fmt.Sprintf("%v", tok),
		"%+v": fmt.Sprintf("%+v", wrapper{tok}),
		"%#v": fmt.Sprintf("%#v", wrapper{tok}),
		"%q":  fmt.Sprintf("%q", tok),
		"%x":  fmt.Sprintf("%x", tok),
	}
	b, err := json.Marshal(wrapper{tok})
	if err != nil {
		t.Fatal(err)
	}
	outputs["json"] = string(b)

	var logs bytes.Buffer
	slog.New(slog.NewTextHandler(&logs, nil)).Info("x", "token", tok)
	outputs["slog"] = logs.String()

	for name, out := range outputs {
		if strings.Contains(out, secret) || strings.Contains(out, fmt.Sprintf("%x", secret)) {
			t.Errorf("%s leaked the token: %q", name, out)
		}
		if !strings.Contains(out, "redacted") {
			t.Errorf("%s = %q, want it to say redacted", name, out)
		}
	}
}

func TestTokenReveal(t *testing.T) {
	if got := Token(secret).Reveal(); got != secret {
		t.Errorf("Reveal() = %q, want the token", got)
	}
}
