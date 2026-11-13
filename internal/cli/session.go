package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/cirope/trk-cli/internal/auth"
	"github.com/spf13/cobra"
)

// whoamiJSON is the --json shape of login and whoami.
type whoamiJSON struct {
	User         api.User         `json:"user"`
	Organization api.Organization `json:"organization"`
	Host         string           `json:"host"`
	Session      sessionJSON      `json:"session"`
}

type sessionJSON struct {
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
}

func toWhoamiJSON(host string, me *api.Me) whoamiJSON {
	return whoamiJSON{
		User: me.User, Organization: me.Organization, Host: host,
		Session: sessionJSON{Name: me.Session.Name, ExpiresAt: me.Session.ExpiresAt.UTC()},
	}
}

func newLoginCmd(a *app) *cobra.Command {
	var noBrowser bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign this machine in, by approving in the browser",
		Long: "Sign this machine in to Trk. trk shows a page to open (and opens it when\n" +
			"it can); approve there, picking the organization if you have several.\n" +
			"No password or token is typed. Signing in again replaces the session.",
		Example: "  trk login\n" +
			"  trk login --no-browser                 # over SSH or in a container\n" +
			"  trk login --host https://trk.example.com",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			host, err := a.host()
			if err != nil {
				return err
			}
			c, err := a.client(host, "")
			if err != nil {
				return err
			}
			store, err := a.tokens()
			if err != nil {
				return err
			}
			name, err := a.env.Hostname()
			if err != nil || name == "" {
				name = "unknown"
			}

			grant, err := auth.Login(cmd.Context(), c, name, func(dc *api.DeviceCode) {
				fmt.Fprintf(a.env.Stderr, "Open this page to approve:\n  %s\n\n", dc.VerificationURIComplete)
				fmt.Fprintf(a.env.Stderr, "Check that it shows the code %s.\n", dc.UserCode)
				if !noBrowser {
					_ = a.env.OpenBrowser(dc.VerificationURIComplete)
				}
				a.progress("Waiting for approval...")
			})
			if err != nil {
				a.progress("\n")
				return fmt.Errorf("signing in: %w", err)
			}
			a.progress(" done.\n")

			if err := store.Set(host, grant.AccessToken); err != nil {
				return fmt.Errorf("keeping the session: %w", err)
			}
			if a.flags.host != "" {
				cfg, err := auth.LoadConfig(a.configPath())
				if err == nil && cfg.Host != host {
					cfg.Host = host
					err = cfg.Save(a.configPath())
				}
				if err != nil {
					return fmt.Errorf("saving %s as the default host: %w", host, err)
				}
			}

			c, err = a.client(host, grant.AccessToken)
			if err != nil {
				return err
			}
			me, err := c.Me(cmd.Context())
			if err != nil {
				return err
			}
			if a.flags.json {
				return a.printJSON(toWhoamiJSON(host, me))
			}
			a.result("Signed in as %s (%s).\n", me.User.Name, me.Organization.Name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "never open a browser; only print the page")
	return cmd
}

func newWhoamiCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show who you are signed in as, where, and until when",
		Example: "  trk whoami\n" +
			"  trk whoami --json",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			host, c, err := a.signedIn()
			if err != nil {
				return err
			}
			me, err := c.Me(cmd.Context())
			if err != nil {
				return err
			}
			if a.flags.json {
				return a.printJSON(toWhoamiJSON(host, me))
			}
			exp := me.Session.ExpiresAt.Local()
			a.result("%s <%s>\n", me.User.Name, me.User.Email)
			a.result("Organization:     %s\n", me.Organization.Name)
			a.result("Host:             %s\n", host)
			a.result("Session expires:  %s (%s)\n", exp.Format("Jan 2, 2006 15:04 MST"), relative(exp, a.env.Now()))
			return nil
		},
	}
}

func newLogoutCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke this machine's session and forget it",
		Long: "Revoke the session on the server, then forget it here. When the server\n" +
			"cannot be reached the session is still forgotten here, and trk says so.",
		Example: "  trk logout\n" +
			"  trk logout --host https://trk.example.com",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			host, err := a.host()
			if err != nil {
				return err
			}
			tok, fromEnv, err := a.token(host)
			var ns notSignedIn
			if errors.As(err, &ns) {
				if a.flags.json {
					return a.printJSON(map[string]any{"host": host, "revoked": false, "signed_in": false})
				}
				a.result("Not signed in to %s.\n", host)
				return nil
			}
			if err != nil {
				return err
			}
			c, err := a.client(host, tok)
			if err != nil {
				return err
			}

			revoked := true
			if err := c.RevokeSession(cmd.Context()); err != nil {
				var apiErr *api.Error
				switch {
				case errors.Is(err, api.ErrUnauthorized):
					// Already expired or revoked: nothing left to revoke.
				case errors.As(err, &apiErr):
					revoked = false
					fmt.Fprintf(a.env.Stderr, "trk: the server did not revoke the session (%s); revoke it in Trk's account settings.\n", apiErr.Message)
				default:
					revoked = false
					fmt.Fprintf(a.env.Stderr, "trk: could not reach %s to revoke the session; revoke it in Trk's account settings.\n", host)
				}
			}
			if fromEnv {
				fmt.Fprintln(a.env.Stderr, "The token comes from TRK_TOKEN; unset it to stop using it.")
			} else {
				store, err := a.tokens()
				if err != nil {
					return err
				}
				if err := store.Delete(host); err != nil {
					return err
				}
			}
			if a.flags.json {
				return a.printJSON(map[string]any{"host": host, "revoked": revoked, "signed_in": false})
			}
			a.result("Signed out of %s.\n", host)
			return nil
		},
	}
}

// relative says how far t is from now, for people: "in 3 days", "2 hours ago".
func relative(t, now time.Time) string {
	d := t.Sub(now)
	future := d >= 0
	if !future {
		d = -d
	}
	var s string
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		s = plural(int(d/time.Minute), "minute")
	case d < 48*time.Hour:
		s = plural(int(d/time.Hour), "hour")
	default:
		s = plural(int(d/(24*time.Hour)), "day")
	}
	if future {
		return "in " + s
	}
	return s + " ago"
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
