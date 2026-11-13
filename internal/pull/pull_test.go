package pull

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/google/go-cmp/cmp"
)

const cardID = "BaHtsbhzVDa9"

// fakeSource is a card in memory. Files are keyed by attachment id.
type fakeSource struct {
	markdown  string
	pages     [][]api.Attachment // first page on the card, then by token "p1", "p2"...
	files     map[string]string
	lie       map[string]int64 // attachment id → size the API claims instead
	downloads []string
}

func (f *fakeSource) CardMarkdown(context.Context, string) (string, error) { return f.markdown, nil }

func (f *fakeSource) Card(_ context.Context, id string) (*api.Card, error) {
	c := &api.Card{ID: id}
	if len(f.pages) > 0 {
		c.Attachments = f.claim(f.pages[0])
		if len(f.pages) > 1 {
			tok := "p1"
			c.AttachmentsNextPageToken = &tok
		}
	}
	return c, nil
}

func (f *fakeSource) CardAttachments(_ context.Context, _ string, token string) (*api.AttachmentPage, error) {
	var i int
	if _, err := fmt.Sscanf(token, "p%d", &i); err != nil || i >= len(f.pages) {
		return nil, errors.New("bad token")
	}
	p := &api.AttachmentPage{Items: f.claim(f.pages[i])}
	if i+1 < len(f.pages) {
		tok := fmt.Sprintf("p%d", i+1)
		p.NextPageToken = &tok
	}
	return p, nil
}

func (f *fakeSource) claim(as []api.Attachment) []api.Attachment {
	out := slices.Clone(as)
	for i := range out {
		out[i].ByteSize = int64(len(f.files[out[i].ID]))
		if n, ok := f.lie[out[i].ID]; ok {
			out[i].ByteSize = n
		}
	}
	return out
}

func (f *fakeSource) Download(_ context.Context, id string, size int64, w io.Writer) (int64, error) {
	f.downloads = append(f.downloads, id)
	body := f.files[id]
	if int64(len(body)) != size {
		n, _ := io.WriteString(w, body[:min(int64(len(body)), size)])
		return int64(n), api.ErrSizeMismatch
	}
	n, err := io.WriteString(w, body)
	return int64(n), err
}

func att(id, path string) api.Attachment {
	return api.Attachment{ID: id, Filename: filepath.Base(path), Path: path}
}

// tree lists the files under dir with their contents, "/"-separated.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		b, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPullWritesTheCardAndItsFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".trk", "cards", cardID)
	src := &fakeSource{
		markdown: "# Card\n",
		pages:    [][]api.Attachment{{att("At5tRaCe9Lg2", "attachments/trace.log")}, {att("At7sCrEen2Pg", "attachments/2-trace.log")}},
		files:    map[string]string{"At5tRaCe9Lg2": "first", "At7sCrEen2Pg": "second!"},
	}
	res, err := Pull(context.Background(), src, cardID, Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"card.md":                 "# Card\n",
		"attachments/trace.log":   "first",
		"attachments/2-trace.log": "second!",
	}
	got := tree(t, dir)
	delete(got, manifestName)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("files (-want +got):\n%s", diff)
	}
	wantFiles := []File{
		{Path: "card.md", Bytes: 7, Status: Written},
		{Path: "attachments/trace.log", Bytes: 5, Status: Written},
		{Path: "attachments/2-trace.log", Bytes: 7, Status: Written},
	}
	if diff := cmp.Diff(wantFiles, res.Files); diff != "" {
		t.Errorf("result (-want +got):\n%s", diff)
	}
	if runtime.GOOS != "windows" {
		for p := range want {
			info, _ := os.Stat(filepath.Join(dir, p))
			if info.Mode().Perm()&0o111 != 0 {
				t.Errorf("%s is executable: %v", p, info.Mode())
			}
		}
	}
}

func TestPullAgainUpdates(t *testing.T) {
	dir := t.TempDir()
	src := &fakeSource{
		markdown: "v1",
		pages:    [][]api.Attachment{{att("At5tRaCe9Lg2", "attachments/a.txt"), att("At7sCrEen2Pg", "attachments/b.txt")}},
		files:    map[string]string{"At5tRaCe9Lg2": "aaa", "At7sCrEen2Pg": "bbb", "Nw3wF1Le2Abc": "new"},
	}
	if _, err := Pull(context.Background(), src, cardID, Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	// Someone's own file in the directory is left alone.
	if err := os.WriteFile(filepath.Join(dir, "attachments", "mine.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	src.markdown = "v2"
	src.pages = [][]api.Attachment{{att("At5tRaCe9Lg2", "attachments/a.txt"), att("Nw3wF1Le2Abc", "attachments/c.txt")}}
	src.downloads = nil
	res, err := Pull(context.Background(), src, cardID, Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	got := tree(t, dir)
	delete(got, manifestName)
	want := map[string]string{
		"card.md": "v2", "attachments/a.txt": "aaa", "attachments/c.txt": "new", "attachments/mine.txt": "mine",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("files (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Nw3wF1Le2Abc"}, src.downloads); diff != "" {
		t.Errorf("downloads (-want +got): an unchanged file was downloaded again\n%s", diff)
	}
	if diff := cmp.Diff([]string{"attachments/b.txt"}, res.Removed); diff != "" {
		t.Errorf("removed (-want +got):\n%s", diff)
	}
	if res.Files[1].Status != Unchanged {
		t.Errorf("a.txt status = %v, want unchanged", res.Files[1].Status)
	}
}

func TestPullNoFiles(t *testing.T) {
	dir := t.TempDir()
	src := &fakeSource{
		markdown: "md",
		pages:    [][]api.Attachment{{att("At5tRaCe9Lg2", "attachments/a.txt")}},
		files:    map[string]string{"At5tRaCe9Lg2": "aaa"},
	}
	if _, err := Pull(context.Background(), src, cardID, Options{Dir: dir, NoFiles: true}); err != nil {
		t.Fatal(err)
	}
	got := tree(t, dir)
	delete(got, manifestName)
	if diff := cmp.Diff(map[string]string{"card.md": "md"}, got); diff != "" {
		t.Errorf("files (-want +got):\n%s", diff)
	}
	if len(src.downloads) != 0 {
		t.Errorf("downloaded %v", src.downloads)
	}
}

// Names from the server are untrusted: nothing lands outside the card's
// attachments/ directory, whatever the name.
func TestPullHostileNames(t *testing.T) {
	hostile := []string{
		"../../escape.txt",
		"attachments/../../escape.txt",
		"attachments/../card.md",
		"/etc/escape.txt",
		`attachments\..\..\escape.txt`,
		"attachments/sub/dir.txt",
		"attachments/",
		"attachments/..",
		"attachments/.",
		"C:/escape.txt",
		"attachments/CON",
		"attachments/nul.txt",
		"attachments/a\x00b.txt",
		"attachments/new\nline.txt",
		"attachments/" + strings.Repeat("x", 400) + ".txt",
		"attachments/trailing. ",
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "card")
	src := &fakeSource{markdown: "md", files: map[string]string{}}
	var page []api.Attachment
	for i, name := range hostile {
		id := fmt.Sprintf("Hst%09d", i) // Only shape-checked by the real API.
		page = append(page, api.Attachment{ID: id, Filename: filepath.Base(name), Path: name})
		src.files[id] = fmt.Sprintf("file %d", i)
	}
	src.pages = [][]api.Attachment{page}

	res, err := Pull(context.Background(), src, cardID, Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(parent)
	if len(entries) != 1 {
		t.Errorf("pull wrote outside its directory: %v", entries)
	}
	got := tree(t, dir)
	if got["card.md"] != "md" {
		t.Errorf("card.md = %q, overwritten by an attachment", got["card.md"])
	}
	seen := map[string]bool{}
	for p := range got {
		if p == "card.md" || p == manifestName {
			continue
		}
		name, ok := strings.CutPrefix(p, "attachments/")
		if !ok || strings.Contains(name, "/") {
			t.Errorf("wrote %q, not directly under attachments/", p)
		}
		if name != CleanName(name) || len(name) > maxNameBytes {
			t.Errorf("wrote %q, an unclean name", p)
		}
		seen[name] = true
	}
	if len(seen) != len(hostile) {
		t.Errorf("wrote %d files, want %d (one each, told apart): %v", len(seen), len(hostile), got)
	}
	if len(res.Files) != len(hostile)+1 {
		t.Errorf("reported %d files", len(res.Files))
	}
}

// A symlink planted in the directory (by an earlier pull, or anyone) is
// never followed out of it.
func TestPullPlantedSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	parent := t.TempDir()
	outside := filepath.Join(parent, "outside")
	dir := filepath.Join(parent, "card")
	if err := os.MkdirAll(outside, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "attachments")); err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{
		markdown: "md",
		pages:    [][]api.Attachment{{att("At5tRaCe9Lg2", "attachments/a.txt")}},
		files:    map[string]string{"At5tRaCe9Lg2": "aaa"},
	}
	_, err := Pull(context.Background(), src, cardID, Options{Dir: dir})
	if err == nil {
		t.Error("pull through a planted symlink succeeded")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("wrote outside through the symlink: %v", entries)
	}

	// A symlink in place of card.md is replaced, not followed.
	if err := os.Remove(filepath.Join(dir, "attachments")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "target.md")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "card.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "card.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Pull(context.Background(), src, cardID, Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "keep" {
		t.Errorf("wrote through a symlinked card.md: %q", b)
	}
}

// A file that is not the size the API said is not left looking whole.
func TestPullSizeMismatch(t *testing.T) {
	dir := t.TempDir()
	src := &fakeSource{
		markdown: "md",
		pages:    [][]api.Attachment{{att("At5tRaCe9Lg2", "attachments/a.txt")}},
		files:    map[string]string{"At5tRaCe9Lg2": "aaaa"},
		lie:      map[string]int64{"At5tRaCe9Lg2": 3},
	}
	_, err := Pull(context.Background(), src, cardID, Options{Dir: dir})
	if !errors.Is(err, api.ErrSizeMismatch) {
		t.Errorf("err = %v, want ErrSizeMismatch", err)
	}
	got := tree(t, dir)
	for p := range got {
		if strings.HasPrefix(p, "attachments/") {
			t.Errorf("left %s behind", p)
		}
	}
}

func TestCleanName(t *testing.T) {
	tests := map[string]string{
		"trace.log":        "trace.log",
		"Informe año.pdf":  "Informe año.pdf",
		"../x":             "x",
		"a/b":              "b",
		`a\b`:              "a_b",
		"..":               "file",
		"":                 "file",
		"CON":              "_CON",
		"nul.txt":          "_nul.txt",
		"a\x00b":           "a_b",
		"tab\there":        "tab_here",
		"end. ":            "end",
		"a:b*c?d\"e<f>g|h": "a_b_c_d_e_f_g_h",
	}
	for in, want := range tests {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
	long := CleanName(strings.Repeat("é", 300) + ".log")
	if len(long) > maxNameBytes || !strings.HasSuffix(long, ".log") {
		t.Errorf("long name = %d bytes, %q", len(long), long[len(long)-8:])
	}
}

func FuzzCleanName(f *testing.F) {
	for _, s := range []string{"trace.log", "../../x", "CON", "a\x00b", "é.log", ". ."} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := CleanName(in)
		if out == "" || out == "." || out == ".." || len(out) > maxNameBytes {
			t.Fatalf("CleanName(%q) = %q", in, out)
		}
		if strings.ContainsAny(out, `/\:*?"<>|`) || strings.ContainsFunc(out, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			t.Fatalf("CleanName(%q) = %q has a forbidden character", in, out)
		}
		if strings.HasSuffix(out, ".") || strings.HasSuffix(out, " ") || reserved(out) {
			t.Fatalf("CleanName(%q) = %q is unsafe on Windows", in, out)
		}
		if filepath.Base(out) != out || !filepath.IsLocal(out) {
			t.Fatalf("CleanName(%q) = %q is not a single local name", in, out)
		}
		if CleanName(out) != out {
			t.Fatalf("CleanName is not idempotent on %q", out)
		}
	})
}
