package gitx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo makes a repository with one commit, isolated from the
// developer's git configuration.
func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "README")
	git(t, dir, "commit", "-q", "-m", "first")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestFind(t *testing.T) {
	dir := newRepo(t)
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	r, err := Find(context.Background(), sub)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	got, _ := filepath.EvalSymlinks(r.Root)
	if got != want {
		t.Errorf("root = %s, want %s", got, want)
	}

	if _, err := Find(context.Background(), t.TempDir()); !errors.Is(err, ErrNotRepo) {
		t.Errorf("Find outside a repo = %v, want ErrNotRepo", err)
	}
}

func TestExclude(t *testing.T) {
	dir := newRepo(t)
	r, err := Find(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, false} {
		added, err := r.Exclude("/.trk/")
		if err != nil {
			t.Fatal(err)
		}
		if added != want {
			t.Errorf("Exclude #%d added = %v, want %v", i+1, added, want)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	if strings.Count(string(b), "/.trk/\n") != 1 {
		t.Errorf("exclude = %q", b)
	}
	// It works: git no longer sees the directory.
	if err := os.MkdirAll(filepath.Join(dir, ".trk", "cards"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".trk", "cards", "x"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := git(t, dir, "status", "--porcelain"); out != "" {
		t.Errorf("git status = %q, want .trk/ ignored", out)
	}
	// .gitignore is never touched.
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !errors.Is(err, os.ErrNotExist) {
		t.Error(".gitignore was created")
	}
}

func TestExcludeInAWorktree(t *testing.T) {
	dir := newRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, dir, "worktree", "add", "-q", wt)
	r, err := Find(context.Background(), wt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Exclude("/.trk/"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	if !strings.Contains(string(b), "/.trk/") {
		t.Errorf("worktree exclude not in the common git dir: %q", b)
	}
}

func TestCleanAndBranches(t *testing.T) {
	dir := newRepo(t)
	ctx := context.Background()
	r, _ := Find(ctx, dir)

	if clean, err := r.Clean(ctx); err != nil || !clean {
		t.Errorf("Clean = %v, %v; want true", clean, err)
	}
	// Untracked files do not make a tree dirty for switching branches.
	_ = os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o600)
	if clean, _ := r.Clean(ctx); !clean {
		t.Error("untracked file made the tree dirty")
	}
	_ = os.WriteFile(filepath.Join(dir, "README"), []byte("changed\n"), 0o600)
	if clean, _ := r.Clean(ctx); clean {
		t.Error("a modified file left the tree clean")
	}
	git(t, dir, "checkout", "-q", "README")

	if ok, _ := r.BranchExists(ctx, "trk/card-BaHtsbhzVDa9"); ok {
		t.Error("branch exists before creating it")
	}
	if err := r.CreateBranch(ctx, "trk/card-BaHtsbhzVDa9"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := r.BranchExists(ctx, "trk/card-BaHtsbhzVDa9"); !ok {
		t.Error("branch missing after creating it")
	}
	if cur, _ := r.CurrentBranch(ctx); cur != "trk/card-BaHtsbhzVDa9" {
		t.Errorf("current branch = %q", cur)
	}
	if err := r.SwitchBranch(ctx, "main"); err != nil {
		t.Fatal(err)
	}
	if cur, _ := r.CurrentBranch(ctx); cur != "main" {
		t.Errorf("current branch = %q", cur)
	}
}

func TestHasRemoteFor(t *testing.T) {
	dir := newRepo(t)
	ctx := context.Background()
	r, _ := Find(ctx, dir)
	if ok, _ := r.HasRemoteFor(ctx, "cirope/trk"); ok {
		t.Error("matched with no remotes")
	}
	git(t, dir, "remote", "add", "origin", "git@github.com:Cirope/trk.git")
	git(t, dir, "remote", "add", "fork", "https://github.com/someone/other")
	if ok, _ := r.HasRemoteFor(ctx, "cirope/trk"); !ok {
		t.Error("did not match git@github.com:Cirope/trk.git")
	}
	if ok, _ := r.HasRemoteFor(ctx, "cirope/trk-cli"); ok {
		t.Error("matched another repository")
	}
}

func TestRepoName(t *testing.T) {
	tests := map[string]string{
		"git@github.com:cirope/trk.git":        "cirope/trk",
		"https://github.com/cirope/trk":        "cirope/trk",
		"https://github.com/cirope/trk.git/":   "cirope/trk",
		"ssh://git@github.com:22/cirope/trk":   "cirope/trk",
		"https://gitlab.com/group/sub/project": "sub/project",
		"/local/path/repo":                     "path/repo",
		"nonsense":                             "",
	}
	for in, want := range tests {
		if got := repoName(in); got != want {
			t.Errorf("repoName(%q) = %q, want %q", in, got, want)
		}
	}
}
