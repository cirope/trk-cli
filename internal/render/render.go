// Package render writes cards for people: grouped lists, a card's page,
// dates and colour. JSON output is the commands' business.
package render

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cirope/trk-cli/internal/api"
)

// Style is how to write: with colour or not, and when "now" is (its
// location is the time zone dates are shown in).
type Style struct {
	Color bool
	Now   time.Time
}

const (
	bold   = "1"
	dim    = "2"
	red    = "31"
	yellow = "33"
	green  = "32"
)

func (s Style) paint(code, text string) string {
	if !s.Color || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansi.ReplaceAllString(s, "") }

// A Group is a heading and its cards.
type Group struct {
	Name  string
	Cards []api.Card
}

// GroupCards groups cards the way "My cards" does, by the server's
// due_window: overdue, this week, later, no date. The server's order is
// kept within each group; empty groups are left out.
func GroupCards(cards []api.Card) []Group {
	groups := []Group{{Name: "OVERDUE"}, {Name: "THIS WEEK"}, {Name: "LATER"}, {Name: "NO DATE"}}
	for _, c := range cards {
		i := 2 // A window this CLI does not know yet reads as later.
		switch {
		case c.DueWindow == nil:
			i = 3
		case *c.DueWindow == "overdue":
			i = 0
		case *c.DueWindow == "week":
			i = 1
		}
		groups[i].Cards = append(groups[i].Cards, c)
	}
	var out []Group
	for _, g := range groups {
		if len(g.Cards) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// CardList writes grouped cards as aligned rows.
func CardList(w io.Writer, groups []Group, s Style) {
	if len(groups) == 0 {
		fmt.Fprintln(w, "No cards.")
		return
	}
	type row struct{ cells [5]string }
	var widths [5]int
	rows := make([][]row, len(groups))
	for gi, g := range groups {
		for _, c := range g.Cards {
			r := row{[5]string{c.ID, c.Title, where(c), dueLabel(c, s.Now), extras(c)}}
			for i, cell := range r.cells {
				widths[i] = max(widths[i], utf8.RuneCountInString(cell))
			}
			rows[gi] = append(rows[gi], r)
		}
	}
	for gi, g := range groups {
		fmt.Fprintln(w, s.paint(bold, g.Name))
		for ri, r := range rows[gi] {
			c := g.Cards[ri]
			var line strings.Builder
			line.WriteString("  ")
			for i, cell := range r.cells {
				painted := cell
				switch i {
				case 0:
					painted = s.paint(dim, cell)
				case 3:
					if overdue(c) {
						painted = s.paint(red, cell)
					}
				case 4:
					painted = paintExtras(c, s)
				}
				line.WriteString(painted)
				if i < len(r.cells)-1 {
					line.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)+2))
				}
			}
			fmt.Fprintln(w, strings.TrimRight(line.String(), " "))
		}
	}
}

// Card writes a card as a person reads it.
func Card(w io.Writer, c *api.Card, s Style) {
	fmt.Fprintln(w, s.paint(bold, c.Title))
	fmt.Fprintf(w, "%s · %s\n", s.paint(dim, c.ID), where(*c))
	fmt.Fprintln(w, c.URL)
	fmt.Fprintln(w)

	field := func(name, value string) {
		if value != "" {
			fmt.Fprintf(w, "%-14s%s\n", name+":", value)
		}
	}
	field("Type", deref(c.Type))
	field("Priority", paintPriority(c.Priority, s))
	field("State", stateLabel(c))
	if due, ok := dueDate(*c, s.Now.Location()); ok {
		label := due.Format("Mon Jan 2, 2006") + " (" + daysFrom(due, s.Now) + ")"
		if overdue(*c) {
			label = s.paint(red, label)
		}
		field("Due", label)
	}
	field("Owners", joinNames(c.Owners, func(p api.Person) string { return p.Name }))
	field("Tags", joinNames(c.Tags, func(t api.Tag) string { return t.Name }))
	agent := "on"
	if !c.Agent {
		agent = "off"
	}
	field("Agent", agent)
	if c.Branch != nil {
		field("Branch", *c.Branch)
	}
	if pr := c.PullRequest; pr != nil {
		label := fmt.Sprintf("#%d %s", pr.Number, pr.State)
		if pr.Checks != nil {
			checks := "checks " + *pr.Checks
			switch *pr.Checks {
			case "passing":
				checks = s.paint(green, checks)
			case "failing":
				checks = s.paint(red, checks)
			}
			label += ", " + checks
		}
		field("Pull request", label+"  "+pr.URL)
	}

	if len(c.Customers) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, s.paint(bold, "Customers"))
		for _, cu := range c.Customers {
			if cu.Notes == "" {
				fmt.Fprintf(w, "  %s\n", cu.Name)
				continue
			}
			fmt.Fprintf(w, "  %s: %s\n", cu.Name, indent(cu.Notes, "    "))
		}
	}
	if desc := deref(c.Description); strings.TrimSpace(desc) != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, strings.TrimRight(desc, "\n"))
	}
	if len(c.Messages) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, s.paint(bold, "Conversation"))
		if c.MessagesNextPageToken != nil {
			fmt.Fprintln(w, s.paint(dim, "  Older messages are on the web: trk card show "+c.ID+" --web"))
		}
		for _, m := range c.Messages {
			fmt.Fprintf(w, "  %s\n", s.paint(dim, m.Author+" · "+m.InsertedAt.In(s.Now.Location()).Format("Jan 2 15:04")))
			fmt.Fprintf(w, "  %s\n", indent(strings.TrimRight(m.Body, "\n"), "  "))
		}
	}
	if len(c.Attachments) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, s.paint(bold, "Files"))
		width := 0
		for _, a := range c.Attachments {
			width = max(width, utf8.RuneCountInString(fileLabel(a)))
		}
		for _, a := range c.Attachments {
			label := fileLabel(a)
			pad := strings.Repeat(" ", width-utf8.RuneCountInString(label))
			fmt.Fprintf(w, "  %s%s  %s\n", label, pad, s.paint(dim, Size(a.ByteSize)))
		}
		if c.AttachmentsNextPageToken != nil {
			fmt.Fprintln(w, s.paint(dim, "  More files: trk card pull "+c.ID))
		}
	}
}

// Size is a byte count for people: 35 B, 1.2 KB, 3.4 MB.
func Size(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	f, suffix := float64(n), []string{"KB", "MB", "GB", "TB"}
	i := -1
	for f >= unit && i < len(suffix)-1 {
		f /= unit
		i++
	}
	return fmt.Sprintf("%.1f %s", f, suffix[i])
}

func where(c api.Card) string {
	board := strings.Join(c.Board.Path, " › ")
	if board == "" {
		board = c.Board.Name
	}
	if c.Column.Name == "" {
		return board
	}
	return board + " · " + c.Column.Name
}

func dueLabel(c api.Card, now time.Time) string {
	due, ok := dueDate(c, now.Location())
	if !ok {
		return ""
	}
	if due.Year() != now.Year() {
		return "due " + due.Format("Jan 2, 2006")
	}
	return "due " + due.Format("Jan 2")
}

func extras(c api.Card) string {
	var parts []string
	if c.Priority != "" && c.Priority != "normal" {
		parts = append(parts, c.Priority)
	}
	if l := stateLabel(&c); l != "" && l != "open" {
		parts = append(parts, l)
	}
	return strings.Join(parts, "  ")
}

func paintExtras(c api.Card, s Style) string {
	var parts []string
	if c.Priority != "" && c.Priority != "normal" {
		parts = append(parts, paintPriority(c.Priority, s))
	}
	if l := stateLabel(&c); l != "" && l != "open" {
		parts = append(parts, l)
	}
	return strings.Join(parts, "  ")
}

func paintPriority(p string, s Style) string {
	switch p {
	case "urgent":
		return s.paint(red, p)
	case "high":
		return s.paint(yellow, p)
	}
	return p
}

func stateLabel(c *api.Card) string {
	switch {
	case c.ClosedAt != nil:
		return "closed"
	case c.State == "waiting":
		return "waiting (the agent asked something)"
	case c.State == "review":
		return "review (ready for a person)"
	}
	return c.State
}

func dueDate(c api.Card, loc *time.Location) (time.Time, bool) {
	if c.DueOn == nil {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(time.DateOnly, *c.DueOn, loc)
	return t, err == nil
}

func overdue(c api.Card) bool {
	return c.ClosedAt == nil && c.DueWindow != nil && *c.DueWindow == "overdue"
}

func day(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// daysFrom says how many days away a date is: today, tomorrow, in 3 days,
// 2 days ago.
func daysFrom(date, now time.Time) string {
	n := int(day(date).Sub(day(now)).Hours()+12) / 24 // Rounds across DST.
	switch {
	case n == 0:
		return "today"
	case n == 1:
		return "tomorrow"
	case n == -1:
		return "yesterday"
	case n > 0:
		return fmt.Sprintf("in %d days", n)
	}
	return fmt.Sprintf("%d days ago", -n)
}

func joinNames[T any](items []T, name func(T) string) string {
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = name(it)
	}
	return strings.Join(names, ", ")
}

func indent(s, prefix string) string {
	return strings.ReplaceAll(s, "\n", "\n"+prefix)
}

// fileLabel is how a file is named for people: its name in the card's
// directory (2-trace.log for a second trace.log), else its file name.
func fileLabel(a api.Attachment) string {
	if name, ok := strings.CutPrefix(a.Path, "attachments/"); ok && name != "" {
		return name
	}
	return a.Filename
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
