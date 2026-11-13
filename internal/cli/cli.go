// Package cli holds trk's commands: their flags, arguments and output.
// It stays thin: the work is done by the packages it wires together.
package cli

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/cirope/trk-cli/internal/auth"
	"github.com/cirope/trk-cli/internal/update"
	"github.com/spf13/cobra"
)

// Build describes the binary, as stamped in at build time.
type Build struct {
	Version string
	Commit  string
	Date    string
}

// Env is everything a command may touch outside itself. main fills it
// with the real thing; tests with fakes.
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	Getenv func(string) string
	Build  Build

	ConfigDir   string       // The platform's user config directory; trk uses ConfigDir/trk.
	Keyring     auth.Keyring // nil: tokens go to a file.
	HTTPClient  *http.Client
	OpenBrowser func(url string) error
	Hostname    func() (string, error)
	Now         func() time.Time
	Location    *time.Location // Where dates are shown for people.
	Workdir     func() (string, error)

	StdoutIsTerminal bool
	StderrIsTerminal bool

	CacheDir  string // The platform's user cache directory; "" disables the update notice.
	UpdateURL string // Where to ask for the latest release; "" is update.LatestURL.
}

// Run executes the command line args (without the program name) and
// returns the process exit code.
func Run(ctx context.Context, args []string, env Env) int {
	env = withDefaults(env)
	a := &app{env: env}
	root := newRootCmd(a)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err != nil {
		a.reportError(err)
	}
	a.updateNotice(ctx)
	return exitCode(err)
}

func withDefaults(env Env) Env {
	if env.Stdout == nil {
		env.Stdout = io.Discard
	}
	if env.Stderr == nil {
		env.Stderr = io.Discard
	}
	if env.Getenv == nil {
		env.Getenv = func(string) string { return "" }
	}
	if env.HTTPClient == nil {
		env.HTTPClient = &http.Client{}
	}
	if env.OpenBrowser == nil {
		env.OpenBrowser = func(string) error { return nil }
	}
	if env.Hostname == nil {
		env.Hostname = func() (string, error) { return "unknown", nil }
	}
	if env.Now == nil {
		env.Now = time.Now
	}
	if env.Location == nil {
		env.Location = time.Local
	}
	return env
}

func newRootCmd(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:   "trk <command>",
		Short: "Work Trk cards from your terminal and your own agent",
		Long: "trk takes a card from Trk to your terminal, so you or your coding agent\n" +
			"can work it, and the pull request finds its way back to the card.\n\n" +
			"Exit codes: 0 done, 1 failed, 2 wrong usage, 3 not found, 4 not signed in,\n" +
			"5 refused.",
		Example: "  trk login\n" +
			"  trk card take BaHtsbhzVDa9 --branch",
		Version: fmt.Sprintf("%s (commit %s, built %s)",
			a.env.Build.Version, a.env.Build.Commit, a.env.Build.Date),
		SilenceErrors: true,
		SilenceUsage:  true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{fmt.Errorf("unknown command %q", args[0])}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(*cobra.Command, []string) error {
			a.setup()
			return nil
		},
	}
	root.SetVersionTemplate("trk {{.Version}}\n")
	root.SetOut(a.env.Stdout)
	root.SetErr(a.env.Stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError{err}
	})

	f := root.PersistentFlags()
	f.BoolVar(&a.flags.json, "json", false, "print one JSON document, for scripts and agents")
	f.StringVar(&a.flags.host, "host", "", "the Trk install (default https://trk.hullop.com, or $TRK_HOST)")
	f.BoolVar(&a.flags.noColor, "no-color", false, "no colours (also $NO_COLOR)")
	f.BoolVarP(&a.flags.quiet, "quiet", "q", false, "only errors and the essential result")
	f.BoolVarP(&a.flags.verbose, "verbose", "v", false, "requests and timings on stderr (also $TRK_DEBUG=1)")

	root.AddCommand(newLoginCmd(a), newLogoutCmd(a), newWhoamiCmd(a),
		newCardsCmd(a, "cards"), newCardCmd(a))
	return root
}

// noArgs is cobra.NoArgs as a usage error.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageError{fmt.Errorf("%s takes no arguments, got %q", cmd.CommandPath(), args[0])}
	}
	return nil
}

// oneArg wants exactly one argument, named name in the message.
func oneArg(name string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return usageError{fmt.Errorf("%s needs one %s", cmd.CommandPath(), name)}
		}
		return nil
	}
}

// updateNotice says, at most once a day, when a newer trk exists: only to
// a person at a terminal, never under --json or in a pipe.
func (a *app) updateNotice(ctx context.Context) {
	if a.flags.json || !a.env.StdoutIsTerminal || !a.env.StderrIsTerminal ||
		a.env.CacheDir == "" || a.env.Getenv("TRK_NO_UPDATE_NOTIFIER") != "" {
		return
	}
	u := &update.Checker{
		Current:   a.env.Build.Version,
		StatePath: filepath.Join(a.env.CacheDir, "trk", "update.json"),
		URL:       cmp.Or(a.env.UpdateURL, update.LatestURL),
		HTTP:      a.env.HTTPClient,
		Now:       a.env.Now,
	}
	if n := u.Notice(ctx); n != "" {
		fmt.Fprintf(a.env.Stderr, "\n%s\nUpdate with: brew upgrade trk, or see https://github.com/cirope/trk-cli/releases\n", n)
	}
}
