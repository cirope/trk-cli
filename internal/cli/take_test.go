package cli

import (
	"bytes"
	"testing"

	"github.com/cirope/trk-cli/internal/api"
)

func TestReportSteps(t *testing.T) {
	tests := []struct {
		steps api.TakeSteps
		want  string
	}{
		{api.TakeSteps{Owner: "added", Agent: "turned_off", Moved: "to_doing"},
			"You own it now.\nAgent: off for this card.\nMoved to In progress.\n"},
		{api.TakeSteps{Owner: "already", Agent: "already_off", Moved: "already_there"},
			"You already own it.\nAgent: already off.\nAlready in In progress.\n"},
		{api.TakeSteps{Owner: "added", Agent: "turned_off", Moved: "no_doing_column"},
			"You own it now.\nAgent: off for this card.\nThis board has no In progress column; the card stays in In progress.\n"},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		a := &app{env: withDefaults(Env{Stdout: &out})}
		res := &api.TakeResult{Steps: tt.steps}
		res.Card.Column.Name = "In progress"
		a.reportSteps(res)
		if out.String() != tt.want {
			t.Errorf("%+v:\n got %q\nwant %q", tt.steps, out.String(), tt.want)
		}
	}
}
