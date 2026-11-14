package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/cirope/trk-cli/internal/api"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		terminal, json  bool
		wantOK, wantAsk bool
	}{
		{"yes", "y\n", true, false, true, true},
		{"YES", "YES\n", true, false, true, true},
		{"no", "n\n", true, false, false, true},
		{"enter means no", "\n", true, false, false, true},
		{"no terminal", "y\n", false, false, false, false},
		{"json", "y\n", true, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			a := &app{env: withDefaults(Env{
				Stdin: strings.NewReader(tt.input), Stderr: &stderr,
				StdinIsTerminal: tt.terminal, StderrIsTerminal: tt.terminal,
			})}
			a.flags.json = tt.json
			ok, asked := a.confirm("Merge it?")
			if ok != tt.wantOK || asked != tt.wantAsk {
				t.Errorf("confirm = %v, %v; want %v, %v", ok, asked, tt.wantOK, tt.wantAsk)
			}
			if asked && stderr.String() != "Merge it? [y/N] " {
				t.Errorf("prompt = %q", stderr.String())
			}
		})
	}
}

func TestReportMoveMergedPullRequest(t *testing.T) {
	var out bytes.Buffer
	a := &app{env: withDefaults(Env{Stdout: &out})}
	closed := time.Now()
	a.reportMove(&api.Card{
		Column:      api.Column{Name: "Done", Role: "done"},
		ClosedAt:    &closed,
		PullRequest: &api.PullRequest{Number: 12, State: "merged"},
	})
	want := "Moved to Done: the card is closed.\nPull request #12: merged.\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestReportPosted(t *testing.T) {
	for _, tt := range []struct {
		turn bool
		want string
	}{
		{true, "Posted on BaHtsbhzVDa9. Trk's agent is answering; the card is back in In progress.\n"},
		{false, "Posted a note on BaHtsbhzVDa9; the card stays where it is.\n"},
	} {
		var out bytes.Buffer
		a := &app{env: withDefaults(Env{Stdout: &out})}
		a.reportPosted("BaHtsbhzVDa9", &api.Posted{AgentTurn: tt.turn})
		if out.String() != tt.want {
			t.Errorf("agent turn %v: got %q", tt.turn, out.String())
		}
	}
}
