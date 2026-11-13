package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// run executes the CLI with args and returns its exit code, stdout and
// stderr. It never touches the real environment.
func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := Env{
		Stdout: &stdout,
		Stderr: &stderr,
		Getenv: func(string) string { return "" },
		Build:  Build{Version: "1.2.3", Commit: "abc1234", Date: "2026-11-13T00:00:00Z"},
	}
	code := Run(context.Background(), args, env)
	return code, stdout.String(), stderr.String()
}

func TestVersion(t *testing.T) {
	code, stdout, stderr := run(t, "--version")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr: %s", code, ExitOK, stderr)
	}
	want := "trk 1.2.3 (commit abc1234, built 2026-11-13T00:00:00Z)\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown flag", []string{"--bogus"}, "unknown flag: --bogus"},
		{"unknown command", []string{"frobnicate"}, `unknown command "frobnicate"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := run(t, tt.args...)
			if code != ExitUsage {
				t.Errorf("exit code = %d, want %d", code, ExitUsage)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tt.want)
			}
			if strings.Count(stderr, "Error:") > 0 {
				t.Errorf("stderr = %q, want no cobra \"Error:\" prefix", stderr)
			}
		})
	}
}

func TestHelpGoesToStdout(t *testing.T) {
	code, stdout, stderr := run(t, "--help")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr: %s", code, ExitOK, stderr)
	}
	if !strings.Contains(stdout, "Usage:") || !strings.Contains(stdout, "Examples:") {
		t.Errorf("stdout = %q, want usage with examples", stdout)
	}
}

func TestEveryCommandHasAnExample(t *testing.T) {
	root := newRootCmd(Env{Getenv: func(string) string { return "" }})
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			return
		}
		if strings.TrimSpace(c.Example) == "" {
			t.Errorf("command %q has no example", c.CommandPath())
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}
