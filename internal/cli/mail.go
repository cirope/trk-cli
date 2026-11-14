package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/spf13/cobra"
)

func newMailCmd(a *app) *cobra.Command {
	var q api.MailQuery
	cmd := &cobra.Command{
		Use:   "mail",
		Short: "Review email Trk held instead of filing it",
		Long: "List the email Trk's router held instead of filing it as a card (spam,\n" +
			"no board for it, unsure…), newest first. File a thread on a board with\n" +
			"trk mail file, or discard it with trk mail discard. trk never emails\n" +
			"customers: only a person in the app does.",
		Example: "  trk mail\n" +
			"  trk mail --discarded\n" +
			"  trk mail file Mx7tHrEad2Qx --board Soporte",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{fmt.Errorf("unknown command %q for trk mail", args[0])}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("limit") && (q.Limit < 1 || q.Limit > 100) {
				return usageError{errors.New("--limit must be between 1 and 100")}
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			page, err := c.Mail(cmd.Context(), q)
			if err != nil {
				return err
			}
			if a.flags.json {
				if page.Items == nil {
					page.Items = []api.MailThread{}
				}
				return a.printJSON(page)
			}
			a.mailList(a.env.Stdout, page.Items, q.Discarded)
			if page.NextPageToken != nil {
				a.progress("More mail: trk mail --page-token %s\n", *page.NextPageToken)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&q.Discarded, "discarded", false, "list the discarded mail instead of the held")
	cmd.Flags().IntVar(&q.Limit, "limit", 0, "threads per page, 1 to 100 (default the server's)")
	cmd.Flags().StringVar(&q.PageToken, "page-token", "", "the page to show, as printed after a full page")
	cmd.AddCommand(newMailFileCmd(a), newMailDiscardCmd(a))
	return cmd
}

var reasons = map[string]string{
	"no_board":     "no board for it",
	"card_deleted": "its card was deleted",
}

// mailList writes threads, each as a line and its snippet under it.
func (a *app) mailList(w io.Writer, threads []api.MailThread, discarded bool) {
	if len(threads) == 0 {
		if discarded {
			fmt.Fprintln(w, "No discarded mail.")
		} else {
			fmt.Fprintln(w, "No held mail.")
		}
		return
	}
	loc := a.env.Location
	for _, t := range threads {
		reason := reasons[t.Reason]
		if reason == "" {
			reason = t.Reason
		}
		if t.ReasonText != nil && *t.ReasonText != "" {
			reason += ": " + *t.ReasonText
		}
		from := t.From.Address
		if t.From.Name != "" {
			from = t.From.Name + " <" + t.From.Address + ">"
		}
		when := t.HeldAt.In(loc).Format("Jan 2")
		if t.DiscardedAt != nil {
			when = "discarded " + t.DiscardedAt.In(loc).Format("Jan 2")
		}
		emails := ""
		if t.Emails > 1 {
			emails = fmt.Sprintf(" · %d emails", t.Emails)
		}
		fmt.Fprintf(w, "%s  %s\n", t.ID, t.Subject)
		fmt.Fprintf(w, "  from %s · %s · %s%s\n", from, reason, when, emails)
		if s := strings.TrimSpace(t.Snippet); s != "" {
			fmt.Fprintf(w, "  %s\n", strings.ReplaceAll(s, "\n", " "))
		}
	}
}

func newMailFileCmd(a *app) *cobra.Command {
	var board string
	cmd := &cobra.Command{
		Use:     "file <thread> --board <board>",
		Short:   "File a held email thread as a card on a board",
		Example: "  trk mail file Mx7tHrEad2Qx --board Soporte",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError{fmt.Errorf("%s needs one thread", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if board == "" {
				return usageError{errors.New("trk mail file needs --board: the board's id or name")}
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			card, err := c.FileMail(cmd.Context(), args[0], board)
			if err != nil {
				return err
			}
			switch {
			case a.flags.json:
				return a.printJSON(card)
			case a.flags.quiet:
				a.result("%s\n", card.ID)
			default:
				a.result("Filed as %s in %s\n%s\n", card.ID, where(card), card.URL)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&board, "board", "", "the board to file it on, by id or name (required)")
	return cmd
}

func newMailDiscardCmd(a *app) *cobra.Command {
	var block bool
	cmd := &cobra.Command{
		Use:   "discard <thread>",
		Short: "Discard a held email thread",
		Long: "Discard a held email thread. --block also blocks its sender, so Trk\n" +
			"discards what they send from now on.",
		Example: "  trk mail discard Mx7tHrEad2Qx\n" +
			"  trk mail discard Mx7tHrEad2Qx --block",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError{fmt.Errorf("%s needs one thread", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			t, err := c.DiscardMail(cmd.Context(), args[0], block)
			if err != nil {
				return err
			}
			switch {
			case a.flags.json:
				return a.printJSON(t)
			case a.flags.quiet:
				a.result("%s\n", t.ID)
			case block:
				a.result("Discarded %s and blocked %s.\n", t.ID, t.From.Address)
			default:
				a.result("Discarded %s.\n", t.ID)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&block, "block", false, "also block the sender")
	return cmd
}
