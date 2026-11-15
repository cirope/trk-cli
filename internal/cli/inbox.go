package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/spf13/cobra"
)

func newInboxCmd(a *app) *cobra.Command {
	var snoozed, done bool
	var pageToken string
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "The cards waiting for you, and why",
		Long: "List the cards in your inbox: those Trk routed to you, of your areas,\n" +
			"where someone called you, and so on. --snoozed and --done list the others.\n" +
			"trk inbox done takes a card out; trk inbox snooze brings it back later.",
		Example: "  trk inbox\n" +
			"  trk inbox done BaHtsbhzVDa9\n" +
			"  trk inbox snooze BaHtsbhzVDa9 tomorrow",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{fmt.Errorf("unknown command %q for trk inbox", args[0])}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			state := ""
			switch {
			case snoozed && done:
				return usageError{errors.New("use either --snoozed or --done")}
			case snoozed:
				state = "snoozed"
			case done:
				state = "done"
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			page, err := c.Inbox(cmd.Context(), state, pageToken)
			if err != nil {
				return err
			}
			if a.flags.json {
				if page.Items == nil {
					page.Items = []api.InboxItem{}
				}
				return a.printJSON(page)
			}
			a.inboxList(a.env.Stdout, page.Items)
			if page.NextPageToken != nil {
				a.progress("More: trk inbox --page-token %s\n", *page.NextPageToken)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&snoozed, "snoozed", false, "list the snoozed cards")
	cmd.Flags().BoolVar(&done, "done", false, "list the cards marked done")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "the page to show, as printed after a full page")
	cmd.AddCommand(newInboxDoneCmd(a), newInboxSnoozeCmd(a))
	return cmd
}

var whyLabels = map[string]string{
	"router":   "Trk sent it to you",
	"priority": "its priority",
	"customer": "your customer",
	"review":   "ready for your review",
	"question": "a question for you",
}

// inboxReason says why a card is in the inbox, for people.
func inboxReason(it api.InboxItem) string {
	var parts []string
	for _, w := range it.Reason.Why {
		switch {
		case w == "person" && it.Reason.By != nil:
			parts = append(parts, *it.Reason.By+" called you")
		case w == "person":
			parts = append(parts, "someone called you")
		case w == "area" && it.Reason.Area != nil:
			parts = append(parts, "your area "+*it.Reason.Area)
		case whyLabels[w] != "":
			parts = append(parts, whyLabels[w])
		default:
			parts = append(parts, w)
		}
	}
	return strings.Join(parts, ", ")
}

func (a *app) inboxList(w io.Writer, items []api.InboxItem) {
	if len(items) == 0 {
		fmt.Fprintln(w, "Nothing in your inbox.")
		return
	}
	for _, it := range items {
		line := it.Card.ID + "  " + it.Card.Title + "  " + it.Card.Board.Name
		if it.Card.Priority != "" && it.Card.Priority != "normal" {
			line += "  " + it.Card.Priority
		}
		fmt.Fprintln(w, line)
		detail := inboxReason(it) + " · " + it.TaggedAt.In(a.env.Location).Format("Jan 2 15:04")
		if it.SnoozedUntil != nil {
			detail += " · snoozed until " + it.SnoozedUntil.In(a.env.Location).Format("Jan 2 15:04")
		}
		if !it.Seen {
			detail += " · new"
		}
		fmt.Fprintln(w, "  "+detail)
	}
}

func newInboxDoneCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "done <card>",
		Short:   "Take a card out of your inbox",
		Example: "  trk inbox done BaHtsbhzVDa9",
		Args:    oneCard,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseCardRef(args[0])
			if err != nil {
				return err
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			it, err := c.InboxDone(cmd.Context(), id)
			if err != nil {
				return err
			}
			if a.flags.json {
				return a.printJSON(it)
			}
			a.result("Done: %s is out of your inbox.\n", id)
			return nil
		},
	}
}

func newInboxSnoozeCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "snooze <card> <when>",
		Short: "Snooze a card in your inbox until later",
		Long: "Snooze a card in your inbox: it comes back at <when>, which is\n" +
			"later_today, tomorrow, next_week, or a date or date-time\n" +
			"(2026-12-01, 2026-12-01T09:00:00-03:00).",
		Example: "  trk inbox snooze BaHtsbhzVDa9 tomorrow\n" +
			"  trk inbox snooze BaHtsbhzVDa9 2026-12-01",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageError{fmt.Errorf("%s needs a card and when: later_today, tomorrow, next_week or a date", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseCardRef(args[0])
			if err != nil {
				return err
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			it, err := c.InboxSnooze(cmd.Context(), id, args[1])
			if err != nil {
				return err
			}
			if a.flags.json {
				return a.printJSON(it)
			}
			if it.SnoozedUntil != nil {
				a.result("Snoozed %s until %s.\n", id, it.SnoozedUntil.In(a.env.Location).Format("Jan 2, 2006 15:04"))
			} else {
				a.result("Snoozed %s.\n", id)
			}
			return nil
		},
	}
}
