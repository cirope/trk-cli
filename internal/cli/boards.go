package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/spf13/cobra"
)

func newBoardsCmd(a *app) *cobra.Command {
	var (
		limit     int
		pageToken string
	)
	cmd := &cobra.Command{
		Use:   "boards",
		Short: "List boards, subprojects under their parent",
		Long: "List the organization's boards as a tree, subprojects under their parent,\n" +
			"in the team's order, with each board's id, repository and status.",
		Example: "  trk boards\n" +
			"  trk boards --json | jq -r '.items[] | select(.repository) | .name'",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("limit") && (limit < 1 || limit > 100) {
				return usageError{errors.New("--limit must be between 1 and 100")}
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			page, err := c.Boards(cmd.Context(), pageToken, limit)
			if err != nil {
				return err
			}
			if a.flags.json {
				if page.Items == nil {
					page.Items = []api.BoardInfo{}
				}
				return a.printJSON(page)
			}
			boardTree(a.env.Stdout, page.Items)
			if page.NextPageToken != nil {
				a.progress("More boards: trk boards --page-token %s\n", *page.NextPageToken)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "boards per page, 1 to 100 (default the server's)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "the page to show, as printed after a full page")
	return cmd
}

// boardTree writes boards as a tree: each subproject under its parent,
// siblings in the server's order (never sorted here). A board whose
// parent is not on this page is shown at the top.
func boardTree(w io.Writer, boards []api.BoardInfo) {
	if len(boards) == 0 {
		fmt.Fprintln(w, "No boards.")
		return
	}
	onPage := map[string]bool{}
	for _, b := range boards {
		onPage[b.ID] = true
	}
	children := map[string][]api.BoardInfo{}
	var roots []api.BoardInfo
	for _, b := range boards {
		if b.Parent != nil && onPage[b.Parent.ID] {
			children[b.Parent.ID] = append(children[b.Parent.ID], b)
		} else {
			roots = append(roots, b)
		}
	}
	type row struct{ name, id, repo, status, agent string }
	var rows []row
	var walk func(b api.BoardInfo, depth int)
	walk = func(b api.BoardInfo, depth int) {
		r := row{name: strings.Repeat("  ", depth) + b.Name, id: b.ID}
		if b.Repository != nil {
			r.repo = *b.Repository
		}
		if b.Status != "" && b.Status != "active" {
			r.status = strings.ReplaceAll(b.Status, "_", " ")
		}
		// Only where the board sets its own: inherited time is not
		// repeated on every row.
		if b.AgentTime != nil && (b.AgentMinutes != nil || b.AgentMaxMinutes != nil) {
			r.agent = fmt.Sprintf("agent %d–%d min", b.AgentTime.Minutes, b.AgentTime.MaxMinutes)
		}
		rows = append(rows, r)
		for _, c := range children[b.ID] {
			walk(c, depth+1)
		}
	}
	for _, b := range roots {
		walk(b, 0)
	}
	nameW, repoW, statusW := 0, 0, 0
	for _, r := range rows {
		nameW = max(nameW, utf8.RuneCountInString(r.name))
		repoW = max(repoW, utf8.RuneCountInString(r.repo))
		statusW = max(statusW, utf8.RuneCountInString(r.status))
	}
	for _, r := range rows {
		line := pad(r.name, nameW+3) + r.id
		if r.repo != "" || r.status != "" || r.agent != "" {
			line += "  " + pad(r.repo, repoW+2) + pad(r.status, statusW+2) + r.agent
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
}

func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s)))
}

func newBoardCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "board <command>",
		Short: "Create and change boards",
		Example: "  trk board new CLI --parent Soporte --repo cirope/trk-cli\n" +
			"  trk board edit CLI --detail Tests=\"make check\"",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{fmt.Errorf("unknown command %q for trk board", args[0])}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newBoardNewCmd(a), newBoardEditCmd(a))
	return cmd
}

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// boardFlags are the settable fields of a board, shared by new and edit.
type boardFlags struct {
	parent, repo, language, status string
	details                        []string
	// One area: its name, what it covers and its people.
	area, covers string
	people       []string
	// Trk's agent's minutes a turn, as typed: "" inherits.
	agentMinutes, agentMax string
	cmd                    *cobra.Command
}

func (f *boardFlags) add(cmd *cobra.Command, parentHelp string) {
	f.cmd = cmd
	fl := cmd.Flags()
	fl.StringVar(&f.area, "area", "", "an area of the board, by name: added, or replacing the one of that name")
	fl.StringVar(&f.covers, "covers", "", "with --area: what the area covers")
	fl.StringSliceVar(&f.people, "people", nil, "with --area: who is in charge, comma-separated (me, emails or ids)")
	fl.StringVar(&f.parent, "parent", "", parentHelp)
	fl.StringVar(&f.repo, "repo", "", "its GitHub repository, owner/name")
	fl.StringVar(&f.language, "language", "", "en or es: the language of its Markdown and Trk's agent")
	fl.StringVar(&f.status, "status", "", "active, on_hold or completed")
	fl.StringArrayVar(&f.details, "detail", nil, "a detail for whoever works its cards, Name=Value (repeatable)")
	fl.StringVar(&f.agentMinutes, "agent-minutes", "", "`minutes` Trk's agent starts each turn with (default the boards above say, or 15)")
	fl.StringVar(&f.agentMax, "agent-max-minutes", "", "the most `minutes` a turn can grow to when the agent asks (default the boards above say, or 45)")
}

// agentTime is the agent's minutes the flags give: for each flag given, a
// number, or nil for "" (inherit from the boards above). Flags not given
// are left nil. Only edit may clear.
func (f *boardFlags) agentTime(canClear bool) (minutes, most **int, err error) {
	parse := func(flag, v string) (**int, error) {
		if !f.changed(flag) {
			return nil, nil
		}
		if v == "" {
			if !canClear {
				return nil, usageError{fmt.Errorf("--%s needs a number of minutes", flag)}
			}
			return new(*int), nil
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, usageError{fmt.Errorf(`--%s must be a whole number of minutes, like 20, or "" to inherit`, flag)}
		}
		p := &n
		return &p, nil
	}
	if minutes, err = parse("agent-minutes", f.agentMinutes); err != nil {
		return nil, nil, err
	}
	most, err = parse("agent-max-minutes", f.agentMax)
	return minutes, most, err
}

// check validates the flags given, before asking the server.
func (f *boardFlags) check() ([]api.NameValue, error) {
	err := errors.Join(
		oneOf("language", f.language, "en", "es"),
		oneOf("status", f.status, "active", "on_hold", "completed"),
	)
	if err != nil {
		return nil, usageError{err}
	}
	if f.repo != "" && !repoPattern.MatchString(f.repo) {
		return nil, usageError{fmt.Errorf("--repo must be owner/name, got %q", f.repo)}
	}
	if f.area == "" && (f.changed("covers") || f.changed("people")) {
		return nil, usageError{errors.New("--covers and --people need --area: the area's name")}
	}
	details, err := parseDetails(f.details)
	return details, err
}

func (f *boardFlags) changed(flag string) bool {
	return f.cmd != nil && f.cmd.Flags().Changed(flag)
}

// newArea is the area the flags give, for a new board.
func (f *boardFlags) newArea() []api.Area {
	if f.area == "" {
		return nil
	}
	return []api.Area{{Name: f.area, Covers: f.covers, People: nonNil(f.people)}}
}

// mergeAreas is b's areas with the flags' area put in (replacing the one
// of that name, keeping what the flags leave out) and remove taken out.
func (f *boardFlags) mergeAreas(b *api.BoardInfo, remove []string) ([]api.Area, error) {
	out := append([]api.Area{}, b.Areas...)
	for _, name := range remove {
		i := slices.IndexFunc(out, func(a api.Area) bool { return strings.EqualFold(a.Name, name) })
		if i < 0 {
			return nil, usageError{fmt.Errorf("%s has no area %q", b.Name, name)}
		}
		out = slices.Delete(out, i, i+1)
	}
	if f.area == "" {
		return out, nil
	}
	i := slices.IndexFunc(out, func(a api.Area) bool { return strings.EqualFold(a.Name, f.area) })
	if i < 0 {
		return append(out, f.newArea()...), nil
	}
	a := out[i]
	a.Name = f.area
	if f.changed("covers") {
		a.Covers = f.covers
	}
	if f.changed("people") {
		a.People = nonNil(f.people)
	}
	out[i] = a
	return out, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func parseDetails(raw []string) ([]api.NameValue, error) {
	var out []api.NameValue
	for _, d := range raw {
		name, value, ok := strings.Cut(d, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, usageError{fmt.Errorf("--detail must be Name=Value, got %q", d)}
		}
		out = append(out, api.NameValue{Name: strings.TrimSpace(name), Value: value})
	}
	return out, nil
}

func newBoardNewCmd(a *app) *cobra.Command {
	var f boardFlags
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Create a board, or a subproject of one",
		Long: "Create a board. With --parent it is a subproject of that board (an id or\n" +
			"a name). Prints the new board's id, where it is, and its URL; with -q\n" +
			"only the id.",
		Example: "  trk board new CLI --parent Soporte --repo cirope/trk-cli --detail Docs=SPEC.md\n" +
			"  trk board new \"Implementación Acme\" --parent Producto --language es --status on_hold",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError{fmt.Errorf("%s needs one name (quote it)", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			details, err := f.check()
			if err != nil {
				return err
			}
			minutes, most, err := f.agentTime(false)
			if err != nil {
				return err
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			b, err := c.CreateBoard(cmd.Context(), api.NewBoard{
				Name: args[0], Parent: f.parent, Repo: f.repo,
				Language: f.language, Status: f.status, Details: details, Areas: f.newArea(),
				AgentMinutes: deref(minutes), AgentMaxMinutes: deref(most),
			})
			if err != nil {
				return err
			}
			switch {
			case a.flags.json:
				return a.printJSON(b)
			case a.flags.quiet:
				a.result("%s\n", b.ID)
			default:
				a.result("Created %s: %s\n%s\n", b.ID, boardPath(b), b.URL)
				if minutes != nil || most != nil {
					a.result("%s\n", agentTimeLine(b))
				}
			}
			return nil
		},
	}
	f.add(cmd, "make it a subproject of this board (an id or a name)")
	return cmd
}

func deref(p **int) *int {
	if p == nil {
		return nil
	}
	return *p
}

// agentTimeLine says how long Trk's agent works a turn on b, as in effect.
func agentTimeLine(b *api.BoardInfo) string {
	if b.AgentTime == nil {
		return "Trk's agent: this Trk does not say how long a turn takes."
	}
	return fmt.Sprintf("Trk's agent: %d minutes a turn, up to %d.", b.AgentTime.Minutes, b.AgentTime.MaxMinutes)
}

// boardPath is "Soporte › CLI" for a subproject, the name otherwise.
func boardPath(b *api.BoardInfo) string {
	if b.Parent == nil {
		return b.Name
	}
	return b.Parent.Name + " › " + b.Name
}

// findBoard finds a board by id, else by name ignoring case, reading
// every page of the list.
func findBoard(ctx context.Context, c *api.Client, ref string) (*api.BoardInfo, error) {
	var byName []api.BoardInfo
	token := ""
	for {
		page, err := c.Boards(ctx, token, 0)
		if err != nil {
			return nil, err
		}
		for _, b := range page.Items {
			if b.ID == ref {
				return &b, nil
			}
			if strings.EqualFold(b.Name, ref) {
				byName = append(byName, b)
			}
		}
		if page.NextPageToken == nil {
			break
		}
		token = *page.NextPageToken
	}
	switch len(byName) {
	case 0:
		return nil, fmt.Errorf("no board %q: %w", ref, api.ErrNotFound)
	case 1:
		return &byName[0], nil
	}
	ids := make([]string, len(byName))
	for i, b := range byName {
		ids[i] = b.ID
	}
	return nil, refusedError{fmt.Errorf("%d boards are called %q (%s); use the id", len(byName), ref, strings.Join(ids, ", "))}
}

func newBoardEditCmd(a *app) *cobra.Command {
	var (
		f           boardFlags
		name        string
		remove      []string
		removeAreas []string
	)
	cmd := &cobra.Command{
		Use:   "edit <board>",
		Short: "Change a board: its name, parent, repository, language, status, details, agent time",
		Long: "Change a board, given by id or name. Only what you give changes.\n" +
			"--detail adds a detail or replaces the one of that name, keeping the\n" +
			"others; --remove-detail removes one. --parent \"\" makes it a top-level\n" +
			"board. --agent-minutes and --agent-max-minutes set how long Trk's agent\n" +
			"works a turn on its cards; \"\" goes back to what the boards above say.",
		Example: "  trk board edit CLI --detail Tests=\"make check\" --detail Docs=README.md\n" +
			"  trk board edit Soporte --status on_hold --remove-detail Website\n" +
			"  trk board edit \"Implementación Acme\" --parent \"\"\n" +
			"  trk board edit CLI --agent-minutes 20 --agent-max-minutes 60",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError{fmt.Errorf("%s needs one board", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			add, err := f.check()
			if err != nil {
				return err
			}
			flags := cmd.Flags()
			var u api.BoardUpdate
			set := func(flag string, v *string, dst **string) {
				if flags.Changed(flag) {
					*dst = v
				}
			}
			set("name", &name, &u.Name)
			set("parent", &f.parent, &u.Parent)
			set("repo", &f.repo, &u.Repo)
			set("language", &f.language, &u.Language)
			set("status", &f.status, &u.Status)
			if u.AgentMinutes, u.AgentMaxMinutes, err = f.agentTime(true); err != nil {
				return err
			}
			if u == (api.BoardUpdate{}) && len(add) == 0 && len(remove) == 0 && f.area == "" && len(removeAreas) == 0 {
				return usageError{errors.New("nothing to change: give a flag, see trk board edit --help")}
			}

			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			b, err := findBoard(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if len(add) > 0 || len(remove) > 0 {
				details, err := mergeDetails(b, add, remove)
				if err != nil {
					return err
				}
				u.Details = &details
			}
			if f.area != "" || len(removeAreas) > 0 {
				areas, err := f.mergeAreas(b, removeAreas)
				if err != nil {
					return err
				}
				u.Areas = &areas
			}
			// By id: exact, even if a rename races another board's name.
			updated, err := c.UpdateBoard(cmd.Context(), b.ID, u)
			if err != nil {
				return err
			}
			switch {
			case a.flags.json:
				return a.printJSON(updated)
			case a.flags.quiet:
				a.result("%s\n", updated.ID)
			default:
				a.result("Updated %s: %s\n", updated.ID, boardPath(updated))
				if u.AgentMinutes != nil || u.AgentMaxMinutes != nil {
					a.result("%s\n", agentTimeLine(updated))
				}
			}
			return nil
		},
	}
	f.add(cmd, `move it under this board (an id or a name); "" makes it top-level`)
	cmd.Flags().StringVar(&name, "name", "", "rename it")
	cmd.Flags().StringArrayVar(&remove, "remove-detail", nil, "remove the detail of this name (repeatable)")
	cmd.Flags().StringArrayVar(&removeAreas, "remove-area", nil, "remove the area of this name (repeatable)")
	return cmd
}

// mergeDetails is b's details with add put in (replacing a detail of the
// same name, else at the end) and remove taken out.
func mergeDetails(b *api.BoardInfo, add []api.NameValue, remove []string) ([]api.NameValue, error) {
	out := append([]api.NameValue{}, b.Details...)
	for _, name := range remove {
		i := slices.IndexFunc(out, func(d api.NameValue) bool { return strings.EqualFold(d.Name, name) })
		if i < 0 {
			return nil, usageError{fmt.Errorf("%s has no detail %q", b.Name, name)}
		}
		out = slices.Delete(out, i, i+1)
	}
	for _, d := range add {
		i := slices.IndexFunc(out, func(e api.NameValue) bool { return strings.EqualFold(e.Name, d.Name) })
		if i >= 0 {
			out[i] = d
		} else {
			out = append(out, d)
		}
	}
	return out, nil
}
