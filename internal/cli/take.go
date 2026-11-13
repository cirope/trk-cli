package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/cirope/trk-cli/internal/gitx"
	"github.com/spf13/cobra"
)

// takeJSON is the --json shape of a take.
type takeJSON struct {
	Card          api.Card      `json:"card"`
	Steps         api.TakeSteps `json:"steps"`
	Branch        *string       `json:"branch"`
	BranchCreated bool          `json:"branch_created"`
	Pull          *pullJSON     `json:"pull"`
}

func newTakeCmd(a *app) *cobra.Command {
	var (
		f        pullFlags
		noPull   bool
		mkBranch bool
	)
	cmd := &cobra.Command{
		Use:   "take <card>",
		Short: "Set a card up for work outside Trk, and name its branch",
		Long: "Take a card to work it with your own agent or editor. In this order:\n\n" +
			"  1. Makes you an owner of the card.\n" +
			"  2. Turns Trk's agent off for the card.\n" +
			"  3. Moves it to the board's In progress column (unless it is further).\n" +
			"  4. Pulls the card, as trk card pull does.\n\n" +
			"The agent is turned off before the move, so the move never starts it.\n" +
			"Every step is a no-op when already true, so it is safe to run again.\n\n" +
			"On a board with a repository, a pull request from the branch it names\n" +
			"attaches to the card by itself. --branch creates and switches to it, in a\n" +
			"clean checkout of the board's repository.",
		Example: "  trk card take BaHtsbhzVDa9\n" +
			"  trk card take BaHtsbhzVDa9 --branch\n" +
			"  trk card take BaHtsbhzVDa9 --no-pull --json",
		Args: oneArg("card"),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			id, err := parseCardRef(args[0])
			if err != nil {
				return err
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}

			// Refuse --branch before anything on the board changes.
			var repo *gitx.Repo
			if mkBranch {
				if repo, err = a.checkBranchable(ctx, c, id); err != nil {
					return err
				}
			}

			res, err := c.Take(ctx, id)
			if err != nil {
				return err
			}
			out := takeJSON{Card: res.Card, Steps: res.Steps, Branch: res.Branch}
			if !a.flags.json {
				a.reportSteps(res)
			}

			if !noPull {
				p, err := a.pull(ctx, c, id, f)
				if err != nil {
					return fmt.Errorf("the card is yours, with its agent off; %w; run the same command again to finish", err)
				}
				out.Pull = p
			}

			if res.Branch != nil && *res.Branch != branchFor(id) {
				return fmt.Errorf("the server named an unexpected branch %q for card %s", *res.Branch, id)
			}
			if res.Branch != nil && repo != nil {
				if err := repo.CreateBranch(ctx, *res.Branch); err != nil {
					return fmt.Errorf("creating branch %s: %w", *res.Branch, err)
				}
				out.BranchCreated = true
			}
			if a.flags.json {
				return a.printJSON(out)
			}
			switch {
			case res.Branch == nil:
			case out.BranchCreated:
				a.result("\nSwitched to a new branch %s. A pull request from it finds its card.\n", *res.Branch)
			default:
				a.result("\nWork on this branch, and the pull request finds its card:\n  git switch -c %s\n", *res.Branch)
			}
			return nil
		},
	}
	f.add(cmd)
	cmd.Flags().BoolVar(&noPull, "no-pull", false, "do not pull the card")
	cmd.Flags().BoolVar(&mkBranch, "branch", false, "create and switch to the card's branch in this checkout")
	return cmd
}

func (a *app) reportSteps(res *api.TakeResult) {
	switch res.Steps.Owner {
	case "added":
		a.result("You own it now.\n")
	case "already":
		a.result("You already own it.\n")
	}
	switch res.Steps.Agent {
	case "turned_off":
		a.result("Agent: off for this card.\n")
	case "already_off":
		a.result("Agent: already off.\n")
	}
	switch res.Steps.Moved {
	case "to_doing":
		a.result("Moved to %s.\n", res.Card.Column.Name)
	case "already_there":
		a.result("Already in %s.\n", res.Card.Column.Name)
	case "no_doing_column":
		a.result("This board has no In progress column; the card stays in %s.\n", res.Card.Column.Name)
	}
}

// checkBranchable refuses --branch unless the current directory is a
// clean checkout of the card's repository without the branch yet.
func (a *app) checkBranchable(ctx context.Context, c *api.Client, id string) (*gitx.Repo, error) {
	card, err := c.Card(ctx, id)
	if err != nil {
		return nil, err
	}
	if card.Board.Repository == nil || card.Branch == nil {
		return nil, refusedError{errors.New("the card's board has no repository, so there is no branch to create; run without --branch")}
	}
	if *card.Branch != branchFor(id) {
		return nil, fmt.Errorf("the server named an unexpected branch %q for card %s", *card.Branch, id)
	}
	wd, err := a.env.Workdir()
	if err != nil {
		return nil, err
	}
	repo, err := gitx.Find(ctx, wd)
	if errors.Is(err, gitx.ErrNotRepo) {
		return nil, refusedError{fmt.Errorf("--branch: %s is not a git repository; run it in a checkout of %s", wd, *card.Board.Repository)}
	}
	if err != nil {
		return nil, err
	}
	if ok, err := repo.HasRemoteFor(ctx, *card.Board.Repository); err != nil {
		return nil, err
	} else if !ok {
		return nil, refusedError{fmt.Errorf("--branch: this is not a checkout of %s (no remote points to it)", *card.Board.Repository)}
	}
	if clean, err := repo.Clean(ctx); err != nil {
		return nil, err
	} else if !clean {
		return nil, refusedError{errors.New("--branch: the working tree has uncommitted changes; commit or stash them first")}
	}
	if exists, err := repo.BranchExists(ctx, *card.Branch); err != nil {
		return nil, err
	} else if exists {
		return nil, refusedError{fmt.Errorf("--branch: branch %s already exists; switch to it with: git switch %s", *card.Branch, *card.Branch)}
	}
	return repo, nil
}

// branchFor is the branch whose pull requests attach to card id.
func branchFor(id string) string { return "trk/card-" + id }
