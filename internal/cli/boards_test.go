package cli

import (
	"strings"
	"testing"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/google/go-cmp/cmp"
)

func TestMergeDetails(t *testing.T) {
	b := &api.BoardInfo{Name: "Support", Details: []api.NameValue{
		{Name: "Docs", Value: "old"}, {Name: "Tests", Value: "mix test"}, {Name: "Website", Value: "x"},
	}}
	got, err := mergeDetails(b,
		[]api.NameValue{{Name: "docs", Value: "new"}, {Name: "CI", Value: "make check"}},
		[]string{"website"})
	if err != nil {
		t.Fatal(err)
	}
	want := []api.NameValue{{Name: "docs", Value: "new"}, {Name: "Tests", Value: "mix test"}, {Name: "CI", Value: "make check"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
	if b.Details[0].Value != "old" {
		t.Error("mergeDetails changed the board it read")
	}
	if _, err := mergeDetails(b, nil, []string{"Nope"}); exitCode(err) != ExitUsage {
		t.Errorf("removing a missing detail: %v", err)
	}
}

// A board that sets its own agent time says so; inherited time is not
// repeated on every row.
func TestBoardTreeAgentTime(t *testing.T) {
	twenty, sixty := 20, 60
	var out strings.Builder
	boardTree(&out, []api.BoardInfo{
		{ID: "P", Name: "Product", AgentMaxMinutes: &sixty, AgentTime: &api.AgentTime{Minutes: 15, MaxMinutes: 60}},
		{ID: "S", Name: "Support", Parent: &api.BoardRef{ID: "P"}, AgentMinutes: &twenty, AgentTime: &api.AgentTime{Minutes: 20, MaxMinutes: 60}},
		{ID: "A", Name: "Acme", Parent: &api.BoardRef{ID: "P"}, Status: "on_hold", AgentTime: &api.AgentTime{Minutes: 15, MaxMinutes: 60}},
	})
	want := "Product     P             agent 15–60 min\n" +
		"  Support   S             agent 20–60 min\n" +
		"  Acme      A    on hold\n"
	if diff := cmp.Diff(want, out.String()); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}
