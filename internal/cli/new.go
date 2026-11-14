package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/spf13/cobra"
)

// maxDescription bounds a description read from a file or stdin.
const maxDescription = 1 << 20

func newNewCmd(a *app) *cobra.Command {
	var (
		n        api.NewCard
		descFile string
		due      string
		take     bool
		t        takeOptions
	)
	cmd := &cobra.Command{
		Use:   "new --board <board> <title>",
		Short: "Create a card on a board",
		Long: "Create a card on a board, given by id or name. It goes to the board's To do\n" +
			"column unless --column says otherwise (a name or a role: todo, doing,\n" +
			"review, done). Creating a card never starts Trk's agent.\n\n" +
			"Prints the new card's id and URL; with -q only the id, for scripts.\n" +
			"--take then takes it, as trk card take does.",
		Example: "  trk card new --board Soporte \"CSV export loses accents\" --type bug --priority high\n" +
			"  git log -1 --format=%B | trk card new --board Producto \"Follow up\" --description-file -\n" +
			"  id=$(trk card new -q --board Soporte \"Rework the importer\" --due 2026-12-01)\n" +
			"  trk card new --board Soporte \"Fix the export\" --take",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError{fmt.Errorf("%s needs one title (quote it)", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			n.Title = args[0]
			if n.Board == "" {
				return usageError{errors.New("trk card new needs --board: the board's id or name")}
			}
			if err := errors.Join(
				oneOf("type", n.Type, "question", "feature", "bug", "security"),
				oneOf("priority", n.Priority, "low", "normal", "high", "urgent"),
			); err != nil {
				return usageError{err}
			}
			if due != "" {
				if _, err := time.Parse(time.DateOnly, due); err != nil {
					return usageError{fmt.Errorf("--due must be a date like 2026-12-01, got %q", due)}
				}
				n.DueOn = due
			}
			if descFile != "" {
				if cmd.Flags().Changed("description") {
					return usageError{errors.New("use either --description or --description-file")}
				}
				d, err := a.readText(descFile)
				if err != nil {
					return err
				}
				n.Description = d
			}

			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			card, err := c.CreateCard(cmd.Context(), n)
			if err != nil {
				return err
			}
			switch {
			case take && a.flags.quiet && !a.flags.json:
				// Only the id, so id=$(trk card new -q --take ...) works; the
				// take's own report would end up in the variable.
				a.result("%s\n", card.ID)
				out := a.env.Stdout
				a.env.Stdout = io.Discard
				defer func() { a.env.Stdout = out }()
			case take && !a.flags.json:
				a.result("Created %s in %s\n%s\n\n", card.ID, where(card), card.URL)
			case take:
			case a.flags.json:
				return a.printJSON(card)
			case a.flags.quiet:
				a.result("%s\n", card.ID)
				return nil
			default:
				a.result("Created %s in %s\n%s\n", card.ID, where(card), card.URL)
				return nil
			}
			if err := a.take(cmd.Context(), c, card.ID, t); err != nil {
				return fmt.Errorf("card %s was created; taking it: %w", card.ID, err)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&n.Board, "board", "", "the board, by id or name (required)")
	f.StringVar(&n.Description, "description", "", "the card's description (Markdown)")
	f.StringVar(&descFile, "description-file", "", "read the description from a file; - reads stdin")
	f.StringVar(&n.Type, "type", "", "question, feature, bug or security")
	f.StringVar(&n.Priority, "priority", "", "low, normal, high or urgent")
	f.StringVar(&due, "due", "", "the due date, YYYY-MM-DD")
	f.StringVar(&n.Column, "column", "", "the column, by name or role (default the board's To do)")
	f.BoolVar(&take, "take", false, "then take the card, as trk card take does")
	f.BoolVar(&t.noPull, "no-pull", false, "with --take, do not pull the card")
	t.pull.add(cmd)
	return cmd
}

// where is "Producto › Soporte · To do" for a card.
func where(c *api.Card) string {
	board := strings.Join(c.Board.Path, " › ")
	if board == "" {
		board = c.Board.Name
	}
	if c.Column.Name == "" {
		return board
	}
	return board + " · " + c.Column.Name
}

// readText reads a file, or stdin for "-", up to maxDescription, without
// trailing newlines.
func (a *app) readText(path string) (string, error) {
	var r io.Reader
	if path == "-" {
		if a.env.Stdin == nil {
			return "", errors.New("no stdin to read the description from")
		}
		r = a.env.Stdin
	} else {
		f, err := os.Open(path) //nolint:gosec // A file the person named.
		if err != nil {
			return "", usageError{fmt.Errorf("reading the description: %w", err)}
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	b, err := io.ReadAll(io.LimitReader(r, maxDescription+1))
	if err != nil {
		return "", fmt.Errorf("reading the description: %w", err)
	}
	if len(b) > maxDescription {
		return "", usageError{errors.New("the description is over 1 MB")}
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}
