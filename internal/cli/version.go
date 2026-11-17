package cli

import (
	"errors"
	"fmt"

	"github.com/cirope/trk-cli/internal/update"
	"github.com/spf13/cobra"
)

// versionInfo is trk version --json. Without --check, or for a
// development build, Latest, Outdated and Upgrade are null.
type versionInfo struct {
	Version  string  `json:"version"`
	Commit   string  `json:"commit"`
	Date     string  `json:"date"`
	Latest   *string `json:"latest"`
	Outdated *bool   `json:"outdated"`
	Upgrade  *string `json:"upgrade"` // The way to update, when outdated.
}

func newVersionCmd(a *app) *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show trk's version, and with --check whether a newer one exists",
		Long: "Show trk's version. --check asks for the latest release now (no daily\n" +
			"cache) and says how to update when this one is older. Outdated or not,\n" +
			"it exits 0; a check that could not be made exits 1. Development\n" +
			"builds are not checked.",
		Example: "  trk version\n" +
			"  trk version --check\n" +
			"  trk version --check --json | jq -e '.outdated | not'",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b := a.env.Build
			info := versionInfo{Version: b.Version, Commit: b.Commit, Date: b.Date}
			line := fmt.Sprintf("trk %s (commit %s, built %s)", b.Version, b.Commit, b.Date)
			if check {
				u := a.updateChecker()
				latest, err := u.Latest(cmd.Context())
				a.checked = true
				switch {
				case errors.Is(err, update.ErrDevelopment):
					line = fmt.Sprintf("trk %s is a development build: not checked", b.Version)
				case err != nil:
					return fmt.Errorf("checking for a newer trk: %w", err)
				default:
					outdated := u.Outdated(latest)
					info.Latest, info.Outdated = &latest, &outdated
					line = fmt.Sprintf("trk %s is the latest", b.Version)
					if outdated {
						how := a.howToUpdate()
						info.Upgrade = &how
						line = fmt.Sprintf("trk %s is out (you have %s): %s", latest, b.Version, how)
					}
				}
			}
			if a.flags.json {
				return a.printJSON(info)
			}
			a.result("%s\n", line)
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "also ask whether a newer trk exists")
	return cmd
}
