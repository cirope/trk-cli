package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cirope/trk-cli/internal/api/apitest"
	"github.com/rogpeppe/go-internal/testscript"
)

// TestMain makes the test binary also act as trk for the scripts. It is
// main's own wiring with the parts of the machine that tests must never
// touch, or that would make output vary, replaced: no keychain (so the
// session lands in a file under $WORK), no browser (the URL is logged to
// $WORK/browser.log), a fixed hostname, clock and time zone, and a config
// directory under $WORK.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"trk": func() {
			work := os.Getenv("WORK")
			os.Exit(run(os.Args[1:], deps{
				keyring:     noKeyring{},
				openBrowser: logBrowser(filepath.Join(work, "browser.log")),
				hostname:    func() (string, error) { return "test-host", nil },
				configDir:   func() (string, error) { return filepath.Join(work, "config"), nil },
				cacheDir:    func() (string, error) { return filepath.Join(work, "cache"), nil },
				now:         func() time.Time { return scriptNow },
				location:    time.UTC,
			}))
		},
	})
}

// Scripts run on Friday, 13 November 2026, at noon UTC.
var scriptNow = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

// TestScripts runs trk through every script in testdata/script against a
// fake trk built from the API fixtures, asserting output, exit codes and
// files.
func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(env *testscript.Env) error {
			env.Setenv("HOME", filepath.Join(env.WorkDir, "home"))
			// git, for the scripts that use it, without the developer's config.
			env.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			env.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
				env.Setenv(k+"_NAME", "Test")
				env.Setenv(k+"_EMAIL", "test@example.com")
			}
			return nil
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"exits":    exits,
			"fake-trk": fakeTrk,
		},
	})
}

// exits runs a program and asserts its exit code:
//
//	exits 2 trk --bogus
//
// stdout and stderr are kept for the stdout and stderr commands.
func exits(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) < 2 {
		ts.Fatalf("usage: exits <code> <program> [args...]")
	}
	want, err := strconv.Atoi(args[0])
	if err != nil {
		ts.Fatalf("exits: bad code %q", args[0])
	}
	got := 0
	if err := ts.Exec(args[1], args[2:]...); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			ts.Fatalf("exits: running %s: %v", args[1], err)
		}
		got = ee.ExitCode()
	}
	if got != want {
		ts.Fatalf("exit code = %d, want %d", got, want)
	}
}

// fakeTrk starts a fake trk answering with the named fixtures and points
// TRK_HOST at it:
//
//	fake-trk me.ok card.get.ok
func fakeTrk(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) == 0 {
		ts.Fatalf("usage: fake-trk <fixture>...")
	}
	st := &scriptT{ts: ts}
	srv := apitest.NewServer(st, args...)
	ts.Setenv("TRK_HOST", srv.URL)
	ts.Defer(st.cleanup)
}

// scriptT adapts a script to what apitest needs. The server's complaints
// arrive from other goroutines, so they are kept and logged at the end.
type scriptT struct {
	ts       *testscript.TestScript
	mu       sync.Mutex
	errs     []string
	cleanups []func()
}

func (s *scriptT) Helper() {}

func (s *scriptT) Errorf(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errs = append(s.errs, fmt.Sprintf(format, args...))
}

func (s *scriptT) Fatalf(format string, args ...any) { s.ts.Fatalf(format, args...) }

func (s *scriptT) Cleanup(f func()) { s.cleanups = append(s.cleanups, f) }

func (s *scriptT) cleanup() {
	for _, f := range s.cleanups {
		f()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.errs {
		s.ts.Logf("fake trk: %s", e)
	}
}

// noKeyring is a machine without a keychain.
type noKeyring struct{}

var errNoKeychain = errors.New("no keychain in tests")

func (noKeyring) Get(string, string) (string, error) { return "", errNoKeychain }
func (noKeyring) Set(string, string, string) error   { return errNoKeychain }
func (noKeyring) Delete(string, string) error        { return errNoKeychain }

// logBrowser "opens" a URL by appending it to a file.
func logBrowser(path string) func(string) error {
	return func(url string) error {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(f, url)
		return errors.Join(err, f.Close())
	}
}
