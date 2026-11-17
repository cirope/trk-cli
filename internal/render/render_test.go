package render

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/cirope/trk-cli/internal/api/apitest"
	"github.com/google/go-cmp/cmp"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// golden compares got with testdata/<name>.golden, or rewrites it with
// -update. Golden files are reviewed like code.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Errorf("%s mismatch (-want +got):\n%s", path, diff)
	}
}

// Friday, 13 November 2026, noon in Buenos Aires; the week runs Monday 9
// to Sunday 15.
var (
	loc = time.FixedZone("ART", -3*3600)
	now = time.Date(2026, 11, 13, 12, 0, 0, 0, loc)
)

func fixtureCard(t *testing.T) api.Card {
	t.Helper()
	var c api.Card
	if err := json.Unmarshal(apitest.Load(t, "card.get.ok").Response.Body, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func ptr(s string) *string { return &s }

// listCards is a page of cards as the server orders and windows them.
func listCards(t *testing.T) []api.Card {
	t.Helper()
	base := fixtureCard(t)
	mk := func(id, title, due, window, prio string, mod func(*api.Card)) api.Card {
		c := base
		c.ID, c.Title, c.Priority = id, title, prio
		c.DueOn, c.DueWindow = nil, nil
		if due != "" {
			c.DueOn, c.DueWindow = ptr(due), ptr(window)
		}
		if mod != nil {
			mod(&c)
		}
		return c
	}
	return []api.Card{
		mk("BaHtsbhzVDa9", "CSV export loses accents", "2026-11-10", "overdue", "urgent", func(c *api.Card) {
			c.Column = api.Column{Name: "In progress", Role: "doing"}
		}),
		mk("Qw3rTy7uPa2s", "Today's card", "2026-11-13", "week", "high", nil),
		mk("Xk29LmQp77aB", "Rework the importer", "2026-11-15", "week", "normal", func(c *api.Card) {
			c.Board.Path = []string{"Product", "Acme rollout"}
			c.State = "waiting"
		}),
		mk("Zx8cVb2nMm4k", "Next week", "2026-11-16", "later", "low", nil),
		mk("Ab2cDe3fGh4j", "Next year", "2027-01-20", "later", "normal", func(c *api.Card) {
			now := now
			c.ClosedAt = &now
		}),
		mk("Hj5kLm6nPq7r", "Someday", "", "", "normal", nil),
	}
}

func TestGroupByTheServersWindow(t *testing.T) {
	cards := listCards(t)
	// The server's window wins over any date arithmetic here.
	cards[3].DueWindow = ptr("week")
	var got [][]string
	for _, g := range GroupCards(cards) {
		ids := []string{g.Name}
		for _, c := range g.Cards {
			ids = append(ids, c.ID)
		}
		got = append(got, ids)
	}
	want := [][]string{
		{"OVERDUE", "BaHtsbhzVDa9"},
		{"THIS WEEK", "Qw3rTy7uPa2s", "Xk29LmQp77aB", "Zx8cVb2nMm4k"},
		{"LATER", "Ab2cDe3fGh4j"},
		{"NO DATE", "Hj5kLm6nPq7r"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("groups (-want +got):\n%s", diff)
	}
}

func TestGroupLeavesOutEmptyGroups(t *testing.T) {
	groups := GroupCards(listCards(t)[5:])
	if len(groups) != 1 || groups[0].Name != "NO DATE" {
		t.Errorf("groups = %+v", groups)
	}
}

// An unknown window from a newer server is still shown.
func TestGroupUnknownWindow(t *testing.T) {
	cards := listCards(t)[:1]
	cards[0].DueWindow = ptr("next_quarter")
	groups := GroupCards(cards)
	if len(groups) != 1 || groups[0].Name != "LATER" {
		t.Errorf("groups = %+v", groups)
	}
}

func TestCardList(t *testing.T) {
	for _, color := range []bool{false, true} {
		var b bytes.Buffer
		CardList(&b, GroupCards(listCards(t)), Style{Color: color, Now: now})
		name := "cards"
		if color {
			name = "cards.color"
		}
		golden(t, name, b.Bytes())
	}
}

func TestCardListEmpty(t *testing.T) {
	var b bytes.Buffer
	CardList(&b, nil, Style{Now: now})
	if b.String() != "No cards.\n" {
		t.Errorf("empty list = %q", b.String())
	}
}

func TestCard(t *testing.T) {
	c := fixtureCard(t)
	var b bytes.Buffer
	Card(&b, &c, Style{Now: now})
	golden(t, "card", b.Bytes())
	if !strings.Contains(b.String(), "Board details\n  Docs:    https://docs.example.com (Product)\n") {
		t.Errorf("no board details in:\n%s", b.String())
	}

	// Two files of one name are told apart by their path.
	second := c.Attachments[0]
	second.ID, second.Path = "At7sCrEen2Pg", "attachments/2-trace.log"
	c.Attachments = append(c.Attachments, second)
	c.Type = nil
	b.Reset()
	Card(&b, &c, Style{Now: now})
	if !strings.Contains(b.String(), "  trace.log    35 B\n  2-trace.log  35 B") || strings.Contains(b.String(), "Type:") {
		t.Errorf("card without type, with two trace.log:\n%s", b.String())
	}

	c.PullRequest = &api.PullRequest{URL: "https://github.com/cirope/trk/pull/12", Number: 12, State: "open", Checks: ptr("failing")}
	c.MessagesNextPageToken = ptr("pt_older")
	c.Branch, c.Board.Repository, c.Customers, c.Attachments, c.Description = nil, nil, nil, nil, nil
	b.Reset()
	Card(&b, &c, Style{Now: now})
	golden(t, "card.variant", b.Bytes())
}

func TestColorIsOnlyEscapes(t *testing.T) {
	var plain, colored bytes.Buffer
	CardList(&plain, GroupCards(listCards(t)), Style{Now: now})
	CardList(&colored, GroupCards(listCards(t)), Style{Now: now, Color: true})
	if stripANSI(colored.String()) != plain.String() {
		t.Error("colour changes more than escapes")
	}
	if bytes.Contains(plain.Bytes(), []byte("\x1b[")) {
		t.Error("escapes without colour")
	}
}

// An agent's message from a question turn shows the response it proposes
// for the customer, with its note under it.
func TestCardProposedResponse(t *testing.T) {
	c := fixtureCard(t)
	c.Messages[0].Body = "I kept it short and pointed to the export setting."
	c.Messages[0].ProposedResponse = ptr("Hi! Choose **UTF-8** in the export dialog.")
	var b bytes.Buffer
	Card(&b, &c, Style{Now: now})
	want := "  agent · Nov 13 09:15 · proposed response\n" +
		"  Hi! Choose **UTF-8** in the export dialog.\n" +
		"  Note: I kept it short and pointed to the export setting.\n"
	if !strings.Contains(b.String(), want) {
		t.Errorf("conversation:\n%s\nwant it to contain:\n%s", b.String(), want)
	}
}

// A card that came by email says so, and a customer's message is headed
// by the sender.
func TestCardFromEmail(t *testing.T) {
	c := fixtureCard(t)
	c.Email = &api.CardEmail{From: []string{"bob@shop.example"}, Subject: "Invoice 12 is frozen"}
	c.Messages = []api.Message{{
		Author: "customer", Body: "Why can't we edit invoice 12 any more?",
		From:       &api.Address{Name: "Bob Shop", Address: "bob@shop.example"},
		InsertedAt: time.Date(2026, 11, 13, 12, 30, 0, 0, time.UTC),
	}}
	var b bytes.Buffer
	Card(&b, &c, Style{Now: now})
	for _, want := range []string{
		"Email:        from bob@shop.example: Invoice 12 is frozen\n",
		"  Bob Shop <bob@shop.example> · Nov 13 09:30\n  Why can't we edit invoice 12 any more?\n",
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("card:\n%s\nwant it to contain %q", b.String(), want)
		}
	}
}

func TestCardCommits(t *testing.T) {
	c := fixtureCard(t)
	checks := "passing"
	c.Commits = []api.Commit{
		{Sha: "4f2a9c1e8b7d6a5f", Subject: "Keep accents in the CSV export", Author: api.CommitAuthor{Name: "Ana Ruiz", Login: "anaruiz"}, Closes: true, Checks: &checks},
		{Sha: "9b8c7d6e5f4a3b2c", Subject: "Test the export", Author: api.CommitAuthor{Name: "Ana Ruiz"}},
	}
	var b bytes.Buffer
	Card(&b, &c, Style{Now: now})
	want := "Commits\n  4f2a9c1  Keep accents in the CSV export (anaruiz) · closes the card · checks passing\n  9b8c7d6  Test the export (Ana Ruiz)\n"
	if !strings.Contains(b.String(), want) {
		t.Errorf("card:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestCardAreaAndAuthors(t *testing.T) {
	c := fixtureCard(t)
	c.Area = ptr("Security")
	at := time.Date(2026, 11, 13, 12, 30, 0, 0, time.UTC)
	c.Messages = []api.Message{
		{Author: "human", AuthorName: "Franco Catena", Body: "Can you check the encoding?", Mentions: []string{"Ana Ruiz"}, InsertedAt: at},
		{Author: "system", Body: "Called Ana Ruiz.", InsertedAt: at},
	}
	var b bytes.Buffer
	Card(&b, &c, Style{Now: now})
	for _, want := range []string{
		"Area:         Security\n",
		"  Franco Catena · Nov 13 09:30 · calls Ana Ruiz\n  Can you check the encoding?\n",
		"  Trk · Nov 13 09:30\n  Called Ana Ruiz.\n",
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("card:\n%s\nwant it to contain %q", b.String(), want)
		}
	}
}
