// Command trk is the command-line client for trk. It only wires the real
// environment into internal/cli and exits with the code it returns.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/cirope/trk-cli/internal/cli"
)

// Stamped at build time with -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, os.Args[1:], cli.Env{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Getenv: os.Getenv,
		Build:  cli.Build{Version: version, Commit: commit, Date: date},
	})
	stop()
	os.Exit(code)
}
