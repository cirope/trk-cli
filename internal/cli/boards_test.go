package cli

import (
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
