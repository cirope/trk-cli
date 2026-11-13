package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/cirope/trk-cli/internal/render"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// style is how to render for people on this run.
func (a *app) style() render.Style {
	color := a.env.StdoutIsTerminal && !a.flags.noColor && a.env.Getenv("NO_COLOR") == ""
	return render.Style{Color: color, Now: a.env.Now().In(a.env.Location)}
}

// oneOf checks a flag's value against the allowed ones.
func oneOf(flag, value string, allowed ...string) error {
	if value == "" || slices.Contains(allowed, value) {
		return nil
	}
	return usageError{fmt.Errorf("--%s must be one of %s", flag, strings.Join(allowed, ", "))}
}

func newCardCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "card <command>",
		Short: "Read, download and take cards",
		Example: "  trk card show BaHtsbhzVDa9\n" +
			"  trk card pull BaHtsbhzVDa9\n" +
			"  trk card take BaHtsbhzVDa9 --branch",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{fmt.Errorf("unknown command %q for trk card", args[0])}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newCardsCmd(a, "list"), newShowCmd(a), newPullCmd(a), newTakeCmd(a))
	return cmd
}

func newCardsCmd(a *app, use string) *cobra.Command {
	var q api.CardsQuery
	cmd := &cobra.Command{
		Use:   use,
		Short: "List my open cards, across every board",
		Long: "List your open cards across every board, grouped as \"My cards\" is in\n" +
			"Trk: overdue, this week, later, no date. With --board, that board's\n" +
			"cards, not only yours.",
		Example: "  trk cards\n" +
			"  trk cards --board Soporte --state waiting\n" +
			"  trk cards --json | jq -r '.items[].id'",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := errors.Join(
				oneOf("state", q.State, "open", "review", "waiting"),
				oneOf("type", q.Type, "question", "feature", "bug", "security"),
				oneOf("priority", q.Priority, "low", "normal", "high", "urgent"),
			); err != nil {
				return usageError{err}
			}
			if q.Limit < 1 || q.Limit > 100 {
				return usageError{errors.New("--limit must be between 1 and 100")}
			}
			if q.All && q.Board == "" {
				return usageError{errors.New("--all needs --board: closed cards are listed one board at a time")}
			}
			if !cmd.Flags().Changed("limit") {
				q.Limit = 0 // The server's default, the same 30.
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			page, err := c.Cards(cmd.Context(), q)
			if err != nil {
				return err
			}
			if a.flags.json {
				if page.Items == nil {
					page.Items = []api.Card{}
				}
				return a.printJSON(page)
			}
			render.CardList(a.env.Stdout, render.GroupCards(page.Items), a.style())
			if page.NextPageToken != nil {
				a.progress("More cards: trk %s --page-token %s\n", strings.Join(nextPageArgs(cmd), " "), *page.NextPageToken)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&q.Board, "board", "", "only this board's cards (an id or a name), not only mine")
	f.BoolVar(&q.All, "all", false, "with --board, include closed cards")
	f.StringVar(&q.State, "state", "", "open, review or waiting")
	f.StringVar(&q.Type, "type", "", "question, feature, bug or security")
	f.StringVar(&q.Priority, "priority", "", "low, normal, high or urgent")
	f.IntVar(&q.Limit, "limit", 30, "cards per page, 1 to 100")
	f.StringVar(&q.PageToken, "page-token", "", "the page to show, as printed after a full page")
	return cmd
}

// nextPageArgs repeats the command and the flags given, but the page
// token, so the hint for the next page keeps the same filters.
func nextPageArgs(cmd *cobra.Command) []string {
	args := strings.Fields(strings.TrimPrefix(cmd.CommandPath(), "trk"))
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if f.Name == "page-token" {
			return
		}
		if f.Value.Type() == "bool" {
			args = append(args, "--"+f.Name)
			return
		}
		args = append(args, "--"+f.Name, quoteArg(f.Value.String()))
	})
	return args
}

func quoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " '\"\\$`") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func newShowCmd(a *app) *cobra.Command {
	var web, markdown bool
	cmd := &cobra.Command{
		Use:   "show <card>",
		Short: "Show a card: its fields, description, conversation and files",
		Long: "Show a card as a person reads it. <card> is a card id, or the card's URL\n" +
			"as copied from the browser.",
		Example: "  trk card show BaHtsbhzVDa9\n" +
			"  trk card show https://trk.hullop.com/boards/TyzgACHc57CG/cards/BaHtsbhzVDa9\n" +
			"  trk card show BaHtsbhzVDa9 --markdown | claude -p \"Summarize this card\"",
		Args: oneArg("card"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if web && markdown {
				return usageError{errors.New("use either --web or --markdown")}
			}
			id, err := parseCardRef(args[0])
			if err != nil {
				return err
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			if markdown {
				md, err := c.CardMarkdown(cmd.Context(), id)
				if err != nil {
					return err
				}
				_, err = fmt.Fprint(a.env.Stdout, md)
				return err
			}
			card, err := c.Card(cmd.Context(), id)
			if err != nil {
				return err
			}
			if web {
				a.progress("Opening %s\n", card.URL)
				_ = a.env.OpenBrowser(card.URL)
				return nil
			}
			if a.flags.json {
				return a.printJSON(card)
			}
			render.Card(a.env.Stdout, card, a.style())
			return nil
		},
	}
	cmd.Flags().BoolVar(&web, "web", false, "open the card in the browser instead")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "print the card as the server's Markdown (the web app's Download)")
	return cmd
}
