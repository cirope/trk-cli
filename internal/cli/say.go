package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/spf13/cobra"
)

func newSayCmd(a *app) *cobra.Command {
	var (
		file    string
		attach  []string
		mention []string
	)
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
			"  git log -1 --format=%B | trk card say BaHtsbhzVDa9 --file -\n" +
			"  trk card say BaHtsbhzVDa9 \"The log from this morning.\" --attach morning.log\n" +
			"  trk card say BaHtsbhzVDa9 \"Can you check the encoding?\" --mention ana@example.com",
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
			files, err := checkUploads(attach)
			if err != nil {
				return err
			}
			if strings.TrimSpace(body) == "" && len(files) == 0 {
				return usageError{errors.New("nothing to post: give a message, --file or --attach")}
			}

			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			// A note that mentions people never starts an agent turn; only
			// otherwise is the card read, to warn when one will start.
			if len(mention) == 0 {
				card, err := c.Card(cmd.Context(), id)
				if err != nil {
					return err
				}
				if card.Agent && card.Board.Repository != nil {
					a.progress("Trk's agent is on for this card: it will answer with a new turn.\n")
				}
			}
			posted, err := c.PostMessage(cmd.Context(), id, body, mention, files...)
			if err != nil {
				return err
			}
			if a.flags.json {
				return a.printJSON(posted)
			}
			a.reportPosted(id, posted, mention)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "read the message from a file; - reads stdin")
	cmd.Flags().StringArrayVar(&attach, "attach", nil, "send this file with the message (repeatable; 20 MB each)")
	cmd.Flags().StringArrayVar(&mention, "mention", nil, "call this person to the card: me, an email or an id (repeatable); makes it a note, with no agent turn")
	return cmd
}

// reportPosted says what posting did: an agent turn, or a note.
// asked are the people the command asked Trk to mention.
func (a *app) reportPosted(id string, p *api.Posted, asked []string) {
	// Trk answers with who it actually called; it never calls you for your
	// own message, and calls only members.
	switch called := len(p.Message.Mentions); {
	case len(asked) == 0:
	case called == 0:
		fmt.Fprintln(a.env.Stderr, "trk: noted, but nobody was called: Trk never calls you for your own message, and calls only members.")
	case called < len(asked):
		fmt.Fprintf(a.env.Stderr, "trk: Trk called %d of the %d people named: never you, and only members.\n", called, len(asked))
	}
	with := ""
	switch n := len(p.Attachments); n {
	case 0:
	case 1:
		with = " with 1 file"
	default:
		with = fmt.Sprintf(" with %d files", n)
	}
	if len(p.Message.Mentions) > 0 {
		a.result("Posted a note on %s%s, calling %s.\n", id, with, strings.Join(p.Message.Mentions, ", "))
		return
	}
	if p.AgentTurn {
		a.result("Posted on %s%s. Trk's agent is answering; the card is back in In progress.\n", id, with)
		return
	}
	a.result("Posted a note on %s%s; the card stays where it is.\n", id, with)
}
