package cli

import (
	"fmt"
	"regexp"

	"github.com/spf13/cobra"
)

// prURL matches a GitHub pull request URL, also with /files, /commits, a
// query or a fragment after it.
var prURL = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+/pull/[0-9]+`)

func newLinkCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "link <card> <pull request url>",
		Short: "Link a pull request to a card",
		Long: "Link a pull request of the board's repository to a card, for work done on\n" +
			"a branch other than the card's (a pull request from trk/card-<id> links\n" +
			"itself). The card then shows it, and moving the card to done merges it.",
		Example: "  trk card link BaHtsbhzVDa9 https://github.com/cirope/trk/pull/12",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageError{fmt.Errorf("%s needs a card and a pull request URL", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseCardRef(args[0])
			if err != nil {
				return err
			}
			url := prURL.FindString(args[1])
			if url == "" {
				return usageError{fmt.Errorf("%q is not a GitHub pull request URL (https://github.com/owner/repo/pull/123)", args[1])}
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			card, err := c.LinkPullRequest(cmd.Context(), id, url)
			if err != nil {
				return err
			}
			switch {
			case a.flags.json:
				return a.printJSON(card)
			case card.PullRequest != nil:
				a.result("Linked pull request #%d to %s.\n", card.PullRequest.Number, card.ID)
			default:
				a.result("Linked %s to %s.\n", url, card.ID)
			}
			return nil
		},
	}
}
