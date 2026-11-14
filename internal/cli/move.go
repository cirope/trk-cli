package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/spf13/cobra"
)

const closeWarning = "Moving into the done column closes the card and merges its open pull request."

func newMoveCmd(a *app) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "move <card> <column>",
		Short: "Move a card to another column, by name or role",
		Long: "Move a card to another column of its board, by the column's name or its\n" +
			"role (todo, doing, review, done).\n\n" +
			closeWarning + " trk asks first, or\n" +
			"needs --yes when it cannot ask (no terminal, an agent).",
		Example: "  trk card move BaHtsbhzVDa9 review\n" +
			"  trk card move BaHtsbhzVDa9 \"En revisión\"\n" +
			"  trk card move BaHtsbhzVDa9 done --yes",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageError{fmt.Errorf("%s needs a card and a column", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.move(cmd.Context(), args[0], args[1], yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "move into done without asking: close the card and merge its pull request")
	return cmd
}

func newDoneCmd(a *app) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "done <card>",
		Short: "Close a card: move it to done, merging its pull request",
		Long: "Move a card to its board's done column, as trk card move <card> done does.\n" +
			closeWarning + " trk asks first, or\n" +
			"needs --yes when it cannot ask.",
		Example: "  trk card done BaHtsbhzVDa9\n" +
			"  trk card done BaHtsbhzVDa9 --yes",
		Args: oneCard,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.move(cmd.Context(), args[0], "done", yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "close the card and merge its pull request without asking")
	return cmd
}

// move moves a card. Into done the server wants a confirmation (409
// close_required) unless yes; trk asks for it at a terminal.
func (a *app) move(ctx context.Context, ref, column string, yes bool) error {
	id, err := parseCardRef(ref)
	if err != nil {
		return err
	}
	_, c, err := a.signedIn()
	if err != nil {
		return err
	}
	card, err := c.MoveCard(ctx, id, column, yes)
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Code == "close_required" {
		ok, asked := a.confirm(apiErr.Message)
		switch {
		case !asked:
			return refusedError{fmt.Errorf("%s Not moved; run it again with --yes to do it", apiErr.Message)}
		case !ok:
			return refusedError{errors.New("not moved")}
		}
		card, err = c.MoveCard(ctx, id, column, true)
	}
	if err != nil {
		return err
	}
	if a.flags.json {
		return a.printJSON(card)
	}
	a.reportMove(card)
	return nil
}

// reportMove says where the card went and, when it closed, what became
// of its pull request.
func (a *app) reportMove(card *api.Card) {
	if card.ClosedAt == nil {
		a.result("Moved to %s.\n", card.Column.Name)
		return
	}
	a.result("Moved to %s: the card is closed.\n", card.Column.Name)
	if pr := card.PullRequest; pr != nil {
		a.result("Pull request #%d: %s.\n", pr.Number, pr.State)
	}
}

// confirm asks question on the terminal. asked is false when there is
// no one to ask (stdin or stderr is not a terminal, or --json).
func (a *app) confirm(question string) (ok, asked bool) {
	if a.flags.json || !a.env.StdinIsTerminal || !a.env.StderrIsTerminal || a.env.Stdin == nil {
		return false, false
	}
	fmt.Fprintf(a.env.Stderr, "%s [y/N] ", question)
	line, _ := bufio.NewReader(a.env.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, true
	}
	return false, true
}
