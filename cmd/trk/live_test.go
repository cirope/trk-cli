//go:build live

package main

import (
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestLive drives trk against a real Trk, never in the default run:
//
//	TRK_LIVE_HOST=http://localhost:4000 TRK_LIVE_TOKEN=... TRK_LIVE_CARD=... \
//	  go test -tags live -run TestLive ./cmd/trk
//
// Use a local Trk with seeded data, never trk.hullop.com. The card is
// only read and pulled; nothing on the board changes.
func TestLive(t *testing.T) {
	host, token, card := os.Getenv("TRK_LIVE_HOST"), os.Getenv("TRK_LIVE_TOKEN"), os.Getenv("TRK_LIVE_CARD")
	if host == "" || token == "" || card == "" {
		t.Skip("set TRK_LIVE_HOST, TRK_LIVE_TOKEN and TRK_LIVE_CARD")
	}
	testscript.Run(t, testscript.Params{
		Dir: "testdata/live",
		Setup: func(env *testscript.Env) error {
			env.Setenv("TRK_HOST", host)
			env.Setenv("TRK_TOKEN", token)
			env.Setenv("CARD", card)
			return nil
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){"exits": exits},
	})
}
