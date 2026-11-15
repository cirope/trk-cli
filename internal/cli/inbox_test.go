package cli

import (
	"bytes"
	"testing"

	"github.com/cirope/trk-cli/internal/api"
)

func TestEmptyInboxNamesTheState(t *testing.T) {
	for state, want := range map[string]string{
		"":        "Nothing waiting in your inbox.\n",
		"snoozed": "No snoozed cards.\n",
		"done":    "No cards marked done.\n",
	} {
		var out bytes.Buffer
		a := &app{env: withDefaults(Env{Stdout: &out})}
		a.inboxList(&out, nil, state)
		if out.String() != want {
			t.Errorf("state %q: got %q, want %q", state, out.String(), want)
		}
	}
}

// Asked to mention someone, but Trk called no one: say so, not "calling".
func TestMentionThatCalledNoOne(t *testing.T) {
	var out, errOut bytes.Buffer
	a := &app{env: withDefaults(Env{Stdout: &out, Stderr: &errOut})}
	a.reportPosted("BaHtsbhzVDa9", &api.Posted{}, []string{"me"})
	if out.String() != "Posted a note on BaHtsbhzVDa9; the card stays where it is.\n" {
		t.Errorf("stdout = %q", out.String())
	}
	if want := "trk: noted, but nobody was called: Trk never calls you for your own message, and calls only members.\n"; errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}

	// Some called, some not.
	out.Reset()
	errOut.Reset()
	posted := &api.Posted{}
	posted.Message.Mentions = []string{"Ana Ruiz"}
	a.reportPosted("BaHtsbhzVDa9", posted, []string{"me", "ana@example.com"})
	if want := "trk: Trk called 1 of the 2 people named: never you, and only members.\n"; errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}
