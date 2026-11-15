package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/spf13/cobra"
)

func newEditCmd(a *app) *cobra.Command {
	var (
		title, desc, descFile, typ, prio, due, area string
	)
	cmd := &cobra.Command{
		Use:   "edit <card>",
		Short: "Change a card's title, description, type, priority, due date or area",
		Long: "Change a card. Only what you give changes. --due \"\" clears the due date.\n" +
			"Its column, owners and agent have their own commands: move, own, take.",
		Example: "  trk card edit BaHtsbhzVDa9 --title \"La exportación a CSV pierde los acentos\"\n" +
			"  trk card edit BaHtsbhzVDa9 --priority urgent --due 2026-12-01\n" +
			"  git log -1 --format=%B | trk card edit BaHtsbhzVDa9 --description-file -",
		Args: oneCard,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseCardRef(args[0])
			if err != nil {
				return err
			}
			flags := cmd.Flags()
			if err := errors.Join(
				oneOf("type", typ, "question", "feature", "bug", "security"),
				oneOf("priority", prio, "low", "normal", "high", "urgent"),
			); err != nil {
				return usageError{err}
			}
			if due != "" {
				if _, err := time.Parse(time.DateOnly, due); err != nil {
					return usageError{fmt.Errorf("--due must be a date like 2026-12-01, got %q", due)}
				}
			}
			if descFile != "" {
				if flags.Changed("description") {
					return usageError{errors.New("use either --description or --description-file")}
				}
				if desc, err = a.readText(descFile); err != nil {
					return err
				}
			}
			var u api.CardUpdate
			set := func(changed bool, v *string, dst **string) {
				if changed {
					*dst = v
				}
			}
			set(flags.Changed("title"), &title, &u.Title)
			set(flags.Changed("description") || descFile != "", &desc, &u.Description)
			set(flags.Changed("type"), &typ, &u.Type)
			set(flags.Changed("priority"), &prio, &u.Priority)
			set(flags.Changed("due"), &due, &u.DueOn)
			set(flags.Changed("area"), &area, &u.Area)
			if u == (api.CardUpdate{}) {
				return usageError{errors.New("nothing to change: give a flag, see trk card edit --help")}
			}

			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			card, err := c.UpdateCard(cmd.Context(), id, u)
			if err != nil {
				return err
			}
			switch {
			case a.flags.json:
				return a.printJSON(card)
			case a.flags.quiet:
				a.result("%s\n", card.ID)
			default:
				a.result("Updated %s: %s\n", card.ID, card.Title)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "the new title")
	f.StringVar(&desc, "description", "", "the new description (Markdown)")
	f.StringVar(&descFile, "description-file", "", "read the description from a file; - reads stdin")
	f.StringVar(&typ, "type", "", "question, feature, bug or security")
	f.StringVar(&prio, "priority", "", "low, normal, high or urgent")
	f.StringVar(&due, "due", "", `the due date, YYYY-MM-DD; "" clears it`)
	f.StringVar(&area, "area", "", `one of the board's areas, by name; "" takes it out of its area`)
	return cmd
}
