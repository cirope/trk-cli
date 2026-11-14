package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/spf13/cobra"
)

func newOwnCmd(a *app) *cobra.Command {
	return ownerCmd(a, "own", "Make you, or others, owners of a card",
		"Make people owners of a card: you when no one is named, or each person\n"+
			"by email or id. Someone who already owns it changes nothing.",
		"  trk card own BaHtsbhzVDa9\n"+
			"  trk card own BaHtsbhzVDa9 ana@example.com me",
		func(ctx context.Context, c *api.Client, id, p string) (*api.Card, error) {
			return c.AddOwner(ctx, id, p)
		})
}

func newUnownCmd(a *app) *cobra.Command {
	return ownerCmd(a, "unown", "Stop owning a card, or take others off it",
		"Take people off a card's owners: you when no one is named, or each person\n"+
			"by email or id. Someone who does not own it changes nothing.",
		"  trk card unown BaHtsbhzVDa9\n"+
			"  trk card unown BaHtsbhzVDa9 ana@example.com",
		func(ctx context.Context, c *api.Client, id, p string) (*api.Card, error) {
			return c.RemoveOwner(ctx, id, p)
		})
}

// ownerCmd is own or unown: change applies to each person in turn.
func ownerCmd(a *app, name, short, long, example string,
	change func(context.Context, *api.Client, string, string) (*api.Card, error)) *cobra.Command {
	return &cobra.Command{
		Use:     name + " <card> [person...]",
		Short:   short,
		Long:    long,
		Example: example,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return usageError{fmt.Errorf("%s needs a card", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseCardRef(args[0])
			if err != nil {
				return err
			}
			people := args[1:]
			if len(people) == 0 {
				people = []string{"me"}
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			var card *api.Card
			for _, p := range people {
				if card, err = change(cmd.Context(), c, id, p); err != nil {
					return err
				}
			}
			if a.flags.json {
				return a.printJSON(card)
			}
			if len(card.Owners) == 0 {
				a.result("No owners now.\n")
				return nil
			}
			names := make([]string, len(card.Owners))
			for i, o := range card.Owners {
				names[i] = o.Name
			}
			a.result("Owners: %s.\n", strings.Join(names, ", "))
			return nil
		},
	}
}
