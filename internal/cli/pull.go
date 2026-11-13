package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/cirope/trk-cli/internal/gitx"
	"github.com/cirope/trk-cli/internal/pull"
	"github.com/cirope/trk-cli/internal/render"
	"github.com/spf13/cobra"
)

// pullJSON is the --json shape of a pull.
type pullJSON struct {
	Card       string      `json:"card"`
	Dir        string      `json:"dir"`
	Files      []pull.File `json:"files"`
	Removed    []string    `json:"removed"`
	GitExclude *string     `json:"git_exclude"`
}

type pullFlags struct {
	dir     string
	noFiles bool
}

func (f *pullFlags) add(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.dir, "dir", "", "write here instead of .trk/cards/<card id>/")
	cmd.Flags().BoolVar(&f.noFiles, "no-files", false, "write only card.md")
}

func newPullCmd(a *app) *cobra.Command {
	var f pullFlags
	cmd := &cobra.Command{
		Use:   "pull <card>",
		Short: "Download a card and its files, for an agent to read",
		Long: "Write the card to .trk/cards/<card id>/ in the current directory: card.md\n" +
			"(the server's Markdown) and its files under attachments/, which card.md\n" +
			"links to. Pulling again updates the directory.\n\n" +
			"Inside a git repository, .trk/ is added to .git/info/exclude so card\n" +
			"content, which can include internal customer notes, is never committed.",
		Example: "  trk card pull BaHtsbhzVDa9\n" +
			"  trk card pull BaHtsbhzVDa9 --dir /tmp/card --no-files",
		Args: oneArg("card"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseCardRef(args[0])
			if err != nil {
				return err
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			out, err := a.pull(cmd.Context(), c, id, f)
			if err != nil {
				return err
			}
			if a.flags.json {
				return a.printJSON(out)
			}
			return nil
		},
	}
	f.add(cmd)
	return cmd
}

// pull writes card id to disk, keeping it out of git, and reports each
// file written on stdout (unless --json).
func (a *app) pull(ctx context.Context, c *api.Client, id string, f pullFlags) (*pullJSON, error) {
	if a.env.Workdir == nil {
		return nil, errors.New("cannot tell the current directory")
	}
	wd, err := a.env.Workdir()
	if err != nil {
		return nil, fmt.Errorf("finding the current directory: %w", err)
	}
	dir, top := filepath.Join(wd, ".trk", "cards", id), filepath.Join(wd, ".trk")
	if f.dir != "" {
		dir = f.dir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(wd, dir)
		}
		top = dir
	}

	// Exclude first, so the card never shows up as untracked.
	exclude, err := a.excludeFromGit(ctx, wd, top)
	if err != nil {
		return nil, err
	}
	res, err := pull.Pull(ctx, c, id, pull.Options{Dir: dir, NoFiles: f.noFiles})
	if err != nil {
		return nil, fmt.Errorf("pulling card %s: %w", id, err)
	}
	if !a.flags.json {
		for _, file := range res.Files {
			if file.Status != pull.Written {
				continue
			}
			p := display(wd, filepath.Join(dir, filepath.FromSlash(file.Path)))
			if file.Path == "card.md" {
				a.result("Wrote %s\n", p)
			} else {
				a.result("Wrote %s (%s)\n", p, render.Size(file.Bytes))
			}
		}
		for _, rel := range res.Removed {
			a.result("Removed %s\n", display(wd, filepath.Join(dir, filepath.FromSlash(rel))))
		}
	}
	return &pullJSON{Card: id, Dir: dir, Files: res.Files, Removed: res.Removed, GitExclude: exclude}, nil
}

// excludeFromGit adds top to .git/info/exclude when wd is in a
// repository and top is inside it. It returns the pattern, if any.
func (a *app) excludeFromGit(ctx context.Context, wd, top string) (*string, error) {
	repo, err := gitx.Find(ctx, wd)
	if errors.Is(err, gitx.ErrNotRepo) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rootReal, err1 := filepath.EvalSymlinks(repo.Root)
	topReal, err2 := evalExisting(top)
	if err1 != nil || err2 != nil {
		return nil, nil
	}
	rel, err := filepath.Rel(rootReal, topReal)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return nil, nil
	}
	pattern := "/" + filepath.ToSlash(rel) + "/"
	added, err := repo.Exclude(pattern)
	if err != nil {
		return nil, fmt.Errorf("keeping card content out of git: %w", err)
	}
	if added {
		a.progress("Added %s to .git/info/exclude, so card content stays out of commits.\n", pattern)
	}
	return &pattern, nil
}

// evalExisting resolves symlinks in the longest existing prefix of p.
func evalExisting(p string) (string, error) {
	var rest []string
	for {
		real, err := filepath.EvalSymlinks(p)
		if err == nil {
			return filepath.Join(append([]string{real}, rest...)...), nil
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", err
		}
		rest = append([]string{filepath.Base(p)}, rest...)
		p = parent
	}
}

// display is p relative to wd when it is inside, else p.
func display(wd, p string) string {
	if rel, err := filepath.Rel(wd, p); err == nil && filepath.IsLocal(rel) {
		return filepath.ToSlash(rel)
	}
	return p
}
