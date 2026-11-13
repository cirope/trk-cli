// Package pull writes a card and its files to a directory, safely: every
// write goes through an os.Root at the card's directory, to a temporary
// file renamed into place, so no name from the server, however crafted,
// can write outside it or leave half a file looking whole.
package pull

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/cirope/trk-cli/internal/api"
)

// Source is the part of the API a pull needs.
type Source interface {
	Card(ctx context.Context, id string) (*api.Card, error)
	CardMarkdown(ctx context.Context, id string) (string, error)
	CardAttachments(ctx context.Context, id, pageToken string) (*api.AttachmentPage, error)
	Download(ctx context.Context, id string, size int64, w io.Writer) (int64, error)
}

// Options says where and what to pull.
type Options struct {
	Dir     string // The card's directory; created if missing.
	NoFiles bool   // Only card.md.
}

// Status is what happened to a file.
type Status string

// What can happen to a file.
const (
	Written   Status = "written"
	Unchanged Status = "unchanged"
)

// File is a file of the pull, by its "/"-separated path in the card's
// directory.
type File struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	Status Status `json:"status"`
}

// Result is what a pull did.
type Result struct {
	Files   []File   `json:"files"`
	Removed []string `json:"removed"`
}

const (
	cardFile     = "card.md"
	filesDir     = "attachments"
	manifestName = ".trk-pull.json"
)

// manifest remembers what an earlier pull wrote, so a pull again
// downloads only new files and deletes only files it wrote itself.
type manifest struct {
	Files map[string]manifestEntry `json:"files"`
}

type manifestEntry struct {
	ID    string `json:"id"`
	Bytes int64  `json:"bytes"`
}

// Pull writes card id to opts.Dir: card.md, and its files under
// attachments/. Pulling again writes new files, skips unchanged ones and
// deletes files no longer on the card.
func Pull(ctx context.Context, src Source, id string, opts Options) (*Result, error) {
	md, err := src.CardMarkdown(ctx, id)
	if err != nil {
		return nil, err
	}
	var attachments []api.Attachment
	if !opts.NoFiles {
		if attachments, err = allAttachments(ctx, src, id); err != nil {
			return nil, err
		}
	}

	if err := os.MkdirAll(opts.Dir, 0o755); err != nil { //nolint:gosec // A working directory, not secret.
		return nil, fmt.Errorf("creating %s: %w", opts.Dir, err)
	}
	root, err := os.OpenRoot(opts.Dir)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", opts.Dir, err)
	}
	defer func() { _ = root.Close() }()

	res := &Result{Files: []File{}, Removed: []string{}}
	if err := writeFile(root, cardFile, func(w io.Writer) error {
		_, err := io.WriteString(w, md)
		return err
	}); err != nil {
		return nil, err
	}
	res.Files = append(res.Files, File{Path: cardFile, Bytes: int64(len(md)), Status: Written})
	if opts.NoFiles {
		return res, nil
	}

	old := readManifest(root)
	next := manifest{Files: map[string]manifestEntry{}}
	if len(attachments) > 0 {
		if err := root.MkdirAll(filesDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating %s: %w", filesDir, err)
		}
	}
	for i, name := range Names(attachments) {
		a := attachments[i]
		rel := filesDir + "/" + name
		entry := manifestEntry{ID: a.ID, Bytes: a.ByteSize}
		next.Files[rel] = entry
		if old.Files[rel] == entry && isRegular(root, rel, a.ByteSize) {
			res.Files = append(res.Files, File{Path: rel, Bytes: a.ByteSize, Status: Unchanged})
			continue
		}
		err := writeFile(root, rel, func(w io.Writer) error {
			_, err := src.Download(ctx, a.ID, a.ByteSize, w)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("writing %s: %w", rel, err)
		}
		res.Files = append(res.Files, File{Path: rel, Bytes: a.ByteSize, Status: Written})
	}

	for rel := range old.Files {
		if _, keep := next.Files[rel]; keep || !isRegular(root, rel, -1) {
			continue
		}
		if err := root.Remove(rel); err != nil {
			return nil, fmt.Errorf("removing %s: %w", rel, err)
		}
		res.Removed = append(res.Removed, rel)
	}
	slices.Sort(res.Removed)

	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeFile(root, manifestName, func(w io.Writer) error {
		_, err := w.Write(b)
		return err
	}); err != nil {
		return nil, err
	}
	return res, nil
}

func allAttachments(ctx context.Context, src Source, id string) ([]api.Attachment, error) {
	card, err := src.Card(ctx, id)
	if err != nil {
		return nil, err
	}
	all := card.Attachments
	for tok := card.AttachmentsNextPageToken; tok != nil; {
		page, err := src.CardAttachments(ctx, id, *tok)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		tok = page.NextPageToken
	}
	return all, nil
}

// Names gives each attachment a clean, distinct file name under
// attachments/, from the server's path for it. Names that clash (also by
// case, for case-insensitive disks) are told apart the way the server's
// zip does: 2-trace.log.
func Names(attachments []api.Attachment) []string {
	taken := map[string]bool{}
	names := make([]string, len(attachments))
	for i, a := range attachments {
		raw := a.Path
		if raw == "" {
			raw = a.Filename
		}
		base := CleanName(strings.TrimPrefix(raw, filesDir+"/"))
		name := base
		for n := 2; taken[strings.ToLower(name)]; n++ {
			name = CleanName(strconv.Itoa(n) + "-" + base)
		}
		taken[strings.ToLower(name)] = true
		names[i] = name
	}
	return names
}

func readManifest(root *os.Root) manifest {
	m := manifest{}
	b, err := root.ReadFile(manifestName)
	if err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

// isRegular reports whether rel is a regular file (not a symlink) of
// size bytes; any size when size is negative.
func isRegular(root *os.Root, rel string, size int64) bool {
	info, err := root.Lstat(rel)
	return err == nil && info.Mode().IsRegular() && (size < 0 || info.Size() == size)
}

// writeFile writes rel through a temporary file beside it, renamed into
// place only when fill succeeded. A symlink at rel is replaced, never
// followed.
func writeFile(root *os.Root, rel string, fill func(io.Writer) error) error {
	tmp := path.Join(path.Dir(rel), ".tmp-"+randomSuffix())
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // Card files are not secret to their reader.
	if err != nil {
		return fmt.Errorf("writing %s: %w", rel, err)
	}
	err = fill(f)
	err = errors.Join(err, f.Close())
	if err == nil {
		err = root.Rename(tmp, rel)
	}
	if err != nil {
		_ = root.Remove(tmp)
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("writing %s: %w", rel, err)
		}
		return err
	}
	return nil
}

func randomSuffix() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
