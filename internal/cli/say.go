package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/spf13/cobra"
)

func newSayCmd(a *app) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:     "say <card> [message]",
		Aliases: []string{"reply"},
		Short:   "Post on a card's conversation",
		Long: "Post a message (Markdown) on a card's conversation, as in the web app.\n\n" +
			"When Trk's agent will answer (a board with a repository and the card's\n" +
			"agent on), the card goes back to In progress and the agent starts a new\n" +
			"turn; trk says so before posting. Take the card first to avoid that.\n" +
			"Otherwise it is a note: the card stays as and where it is.",
		Example: "  trk card say BaHtsbhzVDa9 \"Fixed in #12; the export keeps accents now.\"\n" +
			"  trk card reply BaHtsbhzVDa9 --file notes.md\n" +
			"  git log -1 --format=%B | trk card say BaHtsbhzVDa9 --file -",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 || len(args) > 2 {
				return usageError{fmt.Errorf("%s needs a card, then the message or --file", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseCardRef(args[0])
			if err != nil {
				return err
			}
			var body string
			switch {
			case len(args) == 2 && file != "":
				return usageError{errors.New("use either a message or --file")}
			case file != "":
				if body, err = a.readText(file); err != nil {
					return err
				}
			case len(args) == 2:
				body = args[1]
			}
			if strings.TrimSpace(body) == "" {
				return usageError{errors.New("nothing to post: give a message or --file")}
			}

			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			card, err := c.Card(cmd.Context(), id)
			if err != nil {
				return err
			}
			if card.Agent && card.Board.Repository != nil {
				a.progress("Trk's agent is on for this card: it will answer with a new turn.\n")
			}
			posted, err := c.PostMessage(cmd.Context(), id, body)
			if err != nil {
				return err
			}
			if a.flags.json {
				return a.printJSON(posted)
			}
			a.reportPosted(id, posted)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "read the message from a file; - reads stdin")
	return cmd
}

// reportPosted says what posting did: an agent turn, or a note.
func (a *app) reportPosted(id string, p *api.Posted) {
	if p.AgentTurn {
		a.result("Posted on %s. Trk's agent is answering; the card is back in In progress.\n", id)
		return
	}
	a.result("Posted a note on %s; the card stays where it is.\n", id)
}
