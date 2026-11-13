package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cirope/trk-cli/internal/api/apitest"
	"github.com/google/go-cmp/cmp"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden files")

// The --json output of every command is a public contract: fields are
// added, never renamed or removed. These golden files pin it; a change to
// one is a change to the contract, reviewed as such.
func TestJSONContract(t *testing.T) {
	tests := []struct {
		name     string
		fixtures []string
		args     []string
	}{
		{"whoami", []string{"me.ok"}, []string{"whoami"}},
		{"cards", []string{"cards.list.ok"}, []string{"cards"}},
		{"card_show", []string{"card.get.ok"}, []string{"card", "show", "BaHtsbhzVDa9"}},
		{"card_take", []string{"card.take.ok"}, []string{"card", "take", "BaHtsbhzVDa9", "--no-pull"}},
		{"logout", []string{"session.delete.ok"}, []string{"logout"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := apitest.NewServer(t, tt.fixtures...)
			var stdout, stderr bytes.Buffer
			env := map[string]string{"TRK_HOST": srv.URL, "TRK_TOKEN": "test_access_token"}
			code := Run(context.Background(), append(tt.args, "--json"), Env{
				Stdout: &stdout, Stderr: &stderr,
				Getenv:     func(k string) string { return env[k] },
				HTTPClient: srv.Client(),
				Now:        func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) },
				Workdir:    func() (string, error) { return t.TempDir(), nil },
			})
			if code != ExitOK {
				t.Fatalf("exit %d: %s", code, stderr.String())
			}
			if !json.Valid(stdout.Bytes()) {
				t.Fatalf("stdout is not one JSON document: %q", stdout.String())
			}
			got := bytes.ReplaceAll(stdout.Bytes(), []byte(srv.URL), []byte("http://TRK_HOST"))
			golden(t, filepath.Join("testdata", "json", tt.name+".json"), got)
		})
	}
}

func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Errorf("%s: the --json contract changed (-want +got):\n%s", path, diff)
	}
}
