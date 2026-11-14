package cli

import (
	"fmt"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/spf13/cobra"
)

func newRateCmd(a *app) *cobra.Command {
	var note, noteFile, sent, sentFile string
	cmd := &cobra.Command{
		Use:   "rate <card> good|bad",
		Short: "Rate Trk's agent's latest answer on a card",
		Long: "Record your verdict on Trk's agent's latest answer on a card: good or bad.\n" +
			"For a bad one, say what was wrong (--note) and, if you sent something\n" +
			"else in the end, what (--sent); the agent reads it on its next turn on\n" +
			"the card. Rating the same answer again changes your rating.",
		Example: "  trk card rate BaHtsbhzVDa9 good\n" +
			"  trk card rate BaHtsbhzVDa9 bad --note \"It also happens in LibreOffice.\"\n" +
			"  pbpaste | trk card rate BaHtsbhzVDa9 bad --note \"Too long\" --sent-file -",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageError{fmt.Errorf("%s needs a card and a verdict (good or bad)", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseCardRef(args[0])
			if err != nil {
				return err
			}
			verdict := args[1]
			if verdict != "good" && verdict != "bad" {
				return usageError{fmt.Errorf("the verdict must be good or bad, got %q", verdict)}
			}
			flags := cmd.Flags()
			text := func(flag, value, file string) (string, error) {
				if file == "" {
					return value, nil
				}
				if flags.Changed(flag) {
					return "", usageError{fmt.Errorf("use either --%s or --%s-file", flag, flag)}
				}
				return a.readText(file)
			}
			if note, err = text("note", note, noteFile); err != nil {
				return err
			}
			if sent, err = text("sent", sent, sentFile); err != nil {
				return err
			}

			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			rated, err := c.RateAnswer(cmd.Context(), id, api.Rating{Verdict: verdict, Note: note, Sent: sent})
			if err != nil {
				return err
			}
			if a.flags.json {
				return a.printJSON(rated)
			}
			a.result("Rated the agent's latest answer on %s: %s. So far: %d good, %d bad.\n",
				id, rated.Rating.Verdict, rated.Tally.Good, rated.Tally.Bad)
			if verdict == "bad" {
				a.result("Trk's agent reads this on its next turn on the card.\n")
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&note, "note", "", "what was wrong with the answer")
	f.StringVar(&noteFile, "note-file", "", "read the note from a file; - reads stdin")
	f.StringVar(&sent, "sent", "", "what was sent in the end instead")
	f.StringVar(&sentFile, "sent-file", "", "read what was sent from a file; - reads stdin")
	return cmd
}
