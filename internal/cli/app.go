package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/cirope/trk-cli/internal/auth"
)

const defaultHost = "https://trk.hullop.com"

// app is one run of trk: its environment and global flags.
type app struct {
	env    Env
	flags  globalFlags
	logger *slog.Logger
}

type globalFlags struct {
	json, noColor, quiet, verbose bool
	host                          string
}

// setup applies the global flags, once they are parsed.
func (a *app) setup() {
	if a.env.Getenv("TRK_DEBUG") == "1" {
		a.flags.verbose = true
	}
	level := slog.Level(100) // Off.
	if a.flags.verbose {
		level = slog.LevelDebug
	}
	a.logger = slog.New(slog.NewTextHandler(a.env.Stderr, &slog.HandlerOptions{Level: level}))
}

func (a *app) configPath() string { return filepath.Join(a.env.ConfigDir, "trk", "config.json") }

// host is the install to talk to: --host, $TRK_HOST, the config, or the
// default, without a trailing slash.
func (a *app) host() (string, error) {
	h := a.flags.host
	if h == "" {
		h = a.env.Getenv("TRK_HOST")
	}
	if h == "" && a.env.ConfigDir != "" {
		cfg, err := auth.LoadConfig(a.configPath())
		if err != nil {
			return "", err
		}
		h = cfg.Host
	}
	if h == "" {
		h = defaultHost
	}
	return strings.TrimRight(h, "/"), nil
}

// tokens is where sessions are kept.
func (a *app) tokens() (auth.TokenStore, error) {
	if a.env.ConfigDir == "" {
		return nil, errors.New("cannot find your config directory to keep the session in")
	}
	file := &auth.FileStore{Path: filepath.Join(a.env.ConfigDir, "trk", "credentials.json")}
	if a.env.Keyring == nil {
		return file, nil
	}
	return &auth.Store{
		Keyring: a.env.Keyring,
		File:    file,
		Notice: func(path string) {
			fmt.Fprintf(a.env.Stderr, "No keychain found: the session is kept in %s, readable only by you.\n", path)
		},
	}, nil
}

// notSignedIn is the error for a host without a session.
type notSignedIn struct{ host string }

func (e notSignedIn) Error() string {
	return fmt.Sprintf("not signed in to %s; run trk login", e.host)
}

// token is the session for host: $TRK_TOKEN, or the stored one.
func (a *app) token(host string) (tok api.Token, fromEnv bool, err error) {
	if t := a.env.Getenv("TRK_TOKEN"); t != "" {
		return api.Token(t), true, nil
	}
	store, err := a.tokens()
	if err != nil {
		return "", false, err
	}
	tok, err = store.Get(host)
	if errors.Is(err, auth.ErrNoToken) {
		return "", false, notSignedIn{host}
	}
	return tok, false, err
}

// client returns an API client for host with tok (which may be empty).
func (a *app) client(host string, tok api.Token) (*api.Client, error) {
	c, err := api.New(api.Options{
		Host:       host,
		Token:      tok,
		UserAgent:  fmt.Sprintf("trk-cli/%s (%s; %s)", a.env.Build.Version, runtime.GOOS, runtime.GOARCH),
		HTTPClient: a.env.HTTPClient,
		Logger:     a.logger,
	})
	if err != nil {
		return nil, usageError{err}
	}
	return c, nil
}

// signedIn returns the host and a client with its session.
func (a *app) signedIn() (string, *api.Client, error) {
	host, err := a.host()
	if err != nil {
		return "", nil, err
	}
	tok, _, err := a.token(host)
	if err != nil {
		return "", nil, err
	}
	c, err := a.client(host, tok)
	return host, c, err
}

// printJSON writes v as the one JSON document on stdout.
func (a *app) printJSON(v any) error {
	enc := json.NewEncoder(a.env.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// progress writes to stderr unless --quiet.
func (a *app) progress(format string, args ...any) {
	if !a.flags.quiet {
		fmt.Fprintf(a.env.Stderr, format, args...)
	}
}

// result writes a human result line to stdout.
func (a *app) result(format string, args ...any) {
	fmt.Fprintf(a.env.Stdout, format, args...)
}
