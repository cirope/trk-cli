package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cirope/trk-cli/internal/gitx"

	"github.com/spf13/cobra"
)

// prURL matches a GitHub pull request URL, also with /files, /commits, a
// query or a fragment after it.
var prURL = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+/pull/[0-9]+`)

var (
	commitURL = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+/commit/[0-9a-fA-F]{7,40}$`)
	shaLike   = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
	refLike   = regexp.MustCompile(`^[A-Za-z0-9._/~^@{}-]+$`)
)

// linkTarget says what arg is: a pull request URL ("pull", trimmed to
// the pull request), a commit ("commit": a sha or a commit URL, sent as
// is), a git ref to resolve here ("ref"), or nothing ("").
func linkTarget(arg string) (kind, value string) {
	switch {
	case prURL.MatchString(arg):
		return "pull", prURL.FindString(arg)
	case commitURL.MatchString(arg), shaLike.MatchString(arg):
		return "commit", arg
	case !strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "http") && refLike.MatchString(arg):
		return "ref", arg
	}
	return "", ""
}

func newLinkCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "link <card> <pull request | commit>",
		Short: "Link a pull request or a commit to a card",
		Long: "Link a pull request or a commit of the board's repository to a card, for\n" +
			"work that did not find its card by itself. A pull request from the\n" +
			"branch trk/card-<id> links itself, and so does a commit on the default\n" +
			"branch whose message names the card (#<id>; \"Closes #<id>\" also closes\n" +
			"it). A commit is a sha, a GitHub commit URL, or a git ref here (HEAD, a\n" +
			"branch), resolved to its sha. Linking a commit never closes the card.",
		Example: "  trk card link BaHtsbhzVDa9 https://github.com/cirope/trk/pull/12\n" +
			"  trk card link BaHtsbhzVDa9 HEAD\n" +
			"  trk card link BaHtsbhzVDa9 4f2a9c1",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageError{fmt.Errorf("%s needs a card and a pull request URL or a commit", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseCardRef(args[0])
			if err != nil {
				return err
			}
			kind, value := linkTarget(args[1])
			notOne := usageError{fmt.Errorf("%q is not a pull request URL, a commit or a git ref known here", args[1])}
			if kind == "" {
				return notOne
			}
			if kind == "ref" {
				wd, err := a.env.Workdir()
				if err != nil {
					return err
				}
				repo, err := gitx.Find(cmd.Context(), wd)
				if err != nil {
					return notOne
				}
				if value, err = repo.ResolveCommit(cmd.Context(), value); err != nil {
					return notOne
				}
				kind = "commit"
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			if kind == "commit" {
				cm, err := c.LinkCommit(cmd.Context(), id, value)
				if err != nil {
					return err
				}
				if a.flags.json {
					return a.printJSON(cm)
				}
				short := cm.Sha
				if len(short) > 7 {
					short = short[:7]
				}
				a.result("Linked commit %s (%s) to %s.\n", short, cm.Subject, id)
				return nil
			}
			card, err := c.LinkPullRequest(cmd.Context(), id, value)
			if err != nil {
				return err
			}
			switch {
			case a.flags.json:
				return a.printJSON(card)
			case card.PullRequest != nil:
				a.result("Linked pull request #%d to %s.\n", card.PullRequest.Number, card.ID)
			default:
				a.result("Linked %s to %s.\n", value, card.ID)
			}
			return nil
		},
	}
}
