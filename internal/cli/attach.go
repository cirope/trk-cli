package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/cirope/trk-cli/internal/render"
	"github.com/spf13/cobra"
)

// Trk's limits, checked here so a refused file is never sent.
const (
	maxUpload      = 20 << 20  // Per file.
	maxUploadTotal = 100 << 20 // Per request.
)

// checkUploads checks paths exist, are regular files, not empty, and
// within Trk's limits, and returns them ready to stream.
func checkUploads(paths []string) ([]api.Upload, error) {
	var (
		out   []api.Upload
		total int64
	)
	for _, p := range paths {
		name := filepath.Base(p)
		info, err := os.Stat(p)
		switch {
		case err != nil:
			return nil, usageError{fmt.Errorf("attaching %s: %w", p, err)}
		case !info.Mode().IsRegular():
			return nil, usageError{fmt.Errorf("%s is not a file", p)}
		case info.Size() == 0:
			return nil, usageError{fmt.Errorf("%s is empty", name)}
		case info.Size() > maxUpload:
			return nil, usageError{fmt.Errorf("%s is over the 20 MB a file may be", name)}
		}
		total += info.Size()
		path := p
		out = append(out, api.Upload{Name: name, Size: info.Size(), Open: func() (io.ReadCloser, error) {
			return os.Open(path) //nolint:gosec // A file the person named.
		}})
	}
	if total > maxUploadTotal {
		return nil, usageError{errors.New("the files are over the 100 MB one upload may be; attach them in parts")}
	}
	return out, nil
}

func newAttachCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "attach <card> <file>...",
		Short: "Attach files to a card",
		Long: "Attach files to a card, as the card's Files box in the web app does. Each\n" +
			"file may be up to 20 MB, and one upload up to 100 MB. To send files with\n" +
			"a message, use trk card say --attach.",
		Example: "  trk card attach BaHtsbhzVDa9 trace.log screenshot.png",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return usageError{fmt.Errorf("%s needs a card and at least one file", cmd.CommandPath())}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseCardRef(args[0])
			if err != nil {
				return err
			}
			files, err := checkUploads(args[1:])
			if err != nil {
				return err
			}
			_, c, err := a.signedIn()
			if err != nil {
				return err
			}
			items, err := c.AttachFiles(cmd.Context(), id, files)
			if err != nil {
				return err
			}
			if a.flags.json {
				return a.printJSON(map[string]any{"items": items})
			}
			for _, it := range items {
				if a.flags.quiet {
					a.result("%s\n", it.ID)
				} else {
					a.result("Attached %s (%s) to %s.\n", it.Filename, render.Size(it.ByteSize), id)
				}
			}
			return nil
		},
	}
}
