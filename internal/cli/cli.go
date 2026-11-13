// Package cli holds trk's commands: their flags, arguments and output.
// It stays thin: the work is done by the packages it wires together.
package cli

import (
	"context"
	"fmt"
	"io"

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
	Getenv func(string) string
	Build  Build
}

// Run executes the command line args (without the program name) and
// returns the process exit code.
func Run(ctx context.Context, args []string, env Env) int {
	root := newRootCmd(env)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err != nil {
		reportError(env.Stderr, err)
	}
	return exitCode(err)
}

func newRootCmd(env Env) *cobra.Command {
	root := &cobra.Command{
		Use:   "trk <command>",
		Short: "Work trk cards from your terminal and your own agent",
		Long: "trk takes a card from trk to your terminal, so you or your coding agent\n" +
			"can work it, and the pull request finds its way back to the card.",
		Example: "  trk login\n" +
			"  trk card take BaHtsbhzVDa9 --branch",
		Version: fmt.Sprintf("%s (commit %s, built %s)",
			env.Build.Version, env.Build.Commit, env.Build.Date),
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
	}
	root.SetVersionTemplate("trk {{.Version}}\n")
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError{err}
	})
	return root
}
