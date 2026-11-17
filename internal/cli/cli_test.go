package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cirope/trk-cli/internal/api/apitest"
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
	root := newRootCmd(&app{env: withDefaults(Env{})})
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

func TestUpdateNotice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tag_name":"v9.0.0"}`)
	}))
	defer srv.Close()

	tests := []struct {
		name           string
		stdout, stderr bool // at a terminal
		env            string
		args           []string
		want           bool
	}{
		{"terminal", true, true, "", []string{"--help"}, true},
		{"stdout piped", false, true, "", []string{"--help"}, true},
		{"stderr piped", true, false, "", []string{"--help"}, false},
		{"json", true, true, "", []string{"--help", "--json"}, false},
		{"quiet", true, true, "", []string{"--help", "--quiet"}, false},
		{"opted out", true, true, "1", []string{"--help"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), tt.args, Env{
				Stdout: &stdout, Stderr: &stderr,
				Getenv: func(k string) string {
					if k == "TRK_NO_UPDATE_NOTIFIER" {
						return tt.env
					}
					return ""
				},
				Build:            Build{Version: "1.2.0"},
				CacheDir:         t.TempDir(),
				UpdateURL:        srv.URL,
				Executable:       "/opt/homebrew/Caskroom/trk/1.2.0/trk",
				StdoutIsTerminal: tt.stdout,
				StderrIsTerminal: tt.stderr,
			})
			if code != ExitOK {
				t.Fatalf("exit %d: %s", code, stderr.String())
			}
			got := strings.Contains(stderr.String(), "trk 9.0.0 is out (you have 1.2.0): brew upgrade trk\n")
			if got != tt.want {
				t.Errorf("notice shown = %v, want %v; stderr %q", got, tt.want, stderr.String())
			}
			if strings.Contains(stdout.String(), "is out") {
				t.Error("notice on stdout")
			}
		})
	}
}

func TestHowToUpdate(t *testing.T) {
	tests := []struct{ goos, exe, want string }{
		{"darwin", "/opt/homebrew/Caskroom/trk/0.4.8/trk", "brew upgrade trk"},
		{"darwin", "/usr/local/Caskroom/trk/0.4.8/trk", "brew upgrade trk"},
		{"linux", "/home/linuxbrew/.linuxbrew/Caskroom/trk/0.4.8/trk", "brew upgrade trk"},
		{"linux", "/usr/bin/trk", "upgrade it with your package manager (apt, dnf, zypper, apk)"},
		{"linux", "/usr/local/bin/trk", "curl -fsSL https://packages.hullop.com/trk/install.sh | sh"},
		{"darwin", "/Users/me/bin/trk", "curl -fsSL https://packages.hullop.com/trk/install.sh | sh"},
		{"darwin", "", "curl -fsSL https://packages.hullop.com/trk/install.sh | sh"},
		{"windows", `C:\Tools\trk.exe`, "https://github.com/cirope/trk-cli/releases/latest"},
	}
	for _, tt := range tests {
		if got := howToUpdate(tt.goos, tt.exe); got != tt.want {
			t.Errorf("howToUpdate(%q, %q) = %q, want %q", tt.goos, tt.exe, got, tt.want)
		}
	}
}

func TestErrorsANewerTrkMayFixSaySo(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tag_name":"v9.0.0"}`)
	}))
	defer gh.Close()

	run := func(t *testing.T, version string, optOut bool, fixtures []string, args ...string) (int, string) {
		t.Helper()
		srv := apitest.NewServer(t, fixtures...)
		env := map[string]string{"TRK_HOST": srv.URL, "TRK_TOKEN": "test_access_token"}
		if optOut {
			env["TRK_NO_UPDATE_NOTIFIER"] = "1"
		}
		var stderr bytes.Buffer
		code := Run(context.Background(), args, Env{
			Stderr:     &stderr,
			Getenv:     func(k string) string { return env[k] },
			HTTPClient: srv.Client(),
			Build:      Build{Version: version},
			CacheDir:   t.TempDir(),
			UpdateURL:  gh.URL,
			Executable: "/usr/local/bin/trk",
			GOOS:       "linux",
		})
		return code, stderr.String()
	}
	const want = "Maybe you need trk 9.0.0 (you have 1.2.0): curl -fsSL https://packages.hullop.com/trk/install.sh | sh\n"

	// An agent in a pipe that hits a flag its trk predates hears why.
	code, stderr := run(t, "1.2.0", false, nil, "cards", "--frobnicate")
	if code != ExitUsage || !strings.HasSuffix(stderr, want) {
		t.Errorf("unknown flag: exit %d, stderr %q", code, stderr)
	}
	// So does one whose request the server calls bad (400).
	code, stderr = run(t, "1.2.0", false, []string{"inbox.snooze.bad_until"}, "inbox", "snooze", "BaHtsbhzVDa9", "whenever")
	if code != ExitUsage || !strings.HasSuffix(stderr, want) {
		t.Errorf("400: exit %d, stderr %q", code, stderr)
	}

	// Under --json, the error object gains "hint": {latest, upgrade}.
	_, stderr = run(t, "1.2.0", false, nil, "cards", "--frobnicate", "--json")
	var obj struct {
		Code string
		Hint *struct{ Latest, Upgrade string }
	}
	if err := json.Unmarshal([]byte(stderr), &obj); err != nil {
		t.Fatalf("stderr is not one JSON object: %v: %q", err, stderr)
	}
	if obj.Code != "usage" || obj.Hint == nil || obj.Hint.Latest != "9.0.0" ||
		obj.Hint.Upgrade != "curl -fsSL https://packages.hullop.com/trk/install.sh | sh" {
		t.Errorf("error object = %+v", obj)
	}

	// Up to date, opted out, a development build, or a failure no newer
	// trk fixes (not found): nothing more.
	for _, tc := range []struct {
		version  string
		optOut   bool
		fixtures []string
		args     []string
	}{
		{"9.0.0", false, nil, []string{"cards", "--frobnicate"}},
		{"1.2.0", true, nil, []string{"cards", "--frobnicate"}},
		{"dev", false, nil, []string{"cards", "--frobnicate"}},
		{"1.2.0", false, []string{"card.get.not_found"}, []string{"card", "show", "BaHtsbhzVDa9"}},
	} {
		if _, stderr := run(t, tc.version, tc.optOut, tc.fixtures, tc.args...); strings.Contains(stderr, "Maybe you need") {
			t.Errorf("%s %v: stderr %q", tc.version, tc.args, stderr)
		}
	}
}

func TestVersionCheck(t *testing.T) {
	latest := "v1.3.0"
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = io.WriteString(w, `{"tag_name":"`+latest+`"}`)
	}))
	defer srv.Close()

	run := func(t *testing.T, version, url string, args ...string) (int, string, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), args, Env{
			Stdout: &stdout, Stderr: &stderr,
			Build:      Build{Version: version, Commit: "abc1234", Date: "2026-11-17"},
			CacheDir:   t.TempDir(),
			UpdateURL:  url,
			Executable: "/opt/homebrew/Caskroom/trk/1.2.0/trk",
		})
		return code, stdout.String(), stderr.String()
	}

	// Without --check, the version, like --version.
	code, out, _ := run(t, "1.2.0", srv.URL, "version")
	if code != ExitOK || out != "trk 1.2.0 (commit abc1234, built 2026-11-17)\n" || calls != 0 {
		t.Errorf("version: exit %d, %q, %d calls", code, out, calls)
	}

	// Outdated is still a success; every --check asks, no daily cache.
	for range 2 {
		code, out, _ = run(t, "1.2.0", srv.URL, "version", "--check")
		if code != ExitOK || out != "trk 1.3.0 is out (you have 1.2.0): brew upgrade trk\n" {
			t.Errorf("outdated: exit %d, %q", code, out)
		}
	}
	if calls != 2 {
		t.Errorf("asked %d times for two checks, want 2", calls)
	}
	code, out, _ = run(t, "1.3.0", srv.URL, "version", "--check")
	if code != ExitOK || out != "trk 1.3.0 is the latest\n" {
		t.Errorf("latest: exit %d, %q", code, out)
	}

	// --json: version, latest, outdated, upgrade.
	_, out, _ = run(t, "1.2.0", srv.URL, "version", "--check", "--json")
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got["version"] != "1.2.0" || got["latest"] != "1.3.0" || got["outdated"] != true || got["upgrade"] != "brew upgrade trk" {
		t.Errorf("outdated --json = %v", got)
	}
	_, out, _ = run(t, "1.3.0", srv.URL, "version", "--check", "--json")
	got = nil
	_ = json.Unmarshal([]byte(out), &got)
	if got["outdated"] != false || got["upgrade"] != nil {
		t.Errorf("latest --json = %v", got)
	}

	// A check that fails is an error, not "up to date".
	code, out, errOut := run(t, "1.2.0", "http://127.0.0.1:1", "version", "--check")
	if code != ExitError || out != "" || !strings.Contains(errOut, "checking for a newer trk") {
		t.Errorf("unreachable: exit %d, stdout %q, stderr %q", code, out, errOut)
	}

	// A development build is not checked.
	calls = 0
	code, out, _ = run(t, "dev", srv.URL, "version", "--check", "--json")
	got = nil
	_ = json.Unmarshal([]byte(out), &got)
	if code != ExitOK || calls != 0 || got["version"] != "dev" || got["latest"] != nil || got["outdated"] != nil {
		t.Errorf("dev: exit %d, %d calls, %v", code, calls, got)
	}
}
