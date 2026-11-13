// Package gitx asks git the few questions trk needs: is this a
// repository, is it clean, which remotes, which branches; and keeps
// .trk/ out of commits through .git/info/exclude. It runs the git
// binary.
package gitx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotRepo means the directory is not inside a git repository (or git
// is not installed).
var ErrNotRepo = errors.New("not a git repository")

// Repo is a git working tree.
type Repo struct {
	Root      string // The working tree's top directory.
	commonDir string // The git directory shared by all worktrees.
}

// Find returns the repository dir is in.
func Find(ctx context.Context, dir string) (*Repo, error) {
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel", "--git-common-dir")
	if err != nil {
		return nil, ErrNotRepo
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		return nil, ErrNotRepo
	}
	common := lines[1]
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	return &Repo{Root: filepath.FromSlash(lines[0]), commonDir: common}, nil
}

// Exclude adds pattern to .git/info/exclude (never .gitignore, which
// would be committed) unless it is there; added says whether it was.
func (r *Repo) Exclude(pattern string) (added bool, err error) {
	path := filepath.Join(r.commonDir, "info", "exclude")
	b, err := os.ReadFile(path) //nolint:gosec // git's own exclude file.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == pattern {
			return false, nil
		}
	}
	if len(b) > 0 && !bytes.HasSuffix(b, []byte("\n")) {
		b = append(b, '\n')
	}
	b = append(b, pattern+"\n"...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // git's own layout.
		return false, err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil { //nolint:gosec // git's own file mode.
		return false, fmt.Errorf("writing %s: %w", path, err)
	}
	return true, nil
}

// Clean reports whether tracked files have no changes. Untracked files
// do not count: they do not stop a branch from being created.
func (r *Repo) Clean(ctx context.Context) (bool, error) {
	out, err := run(ctx, r.Root, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// BranchExists reports whether a local branch exists.
func (r *Repo) BranchExists(ctx context.Context, name string) (bool, error) {
	_, err := run(ctx, r.Root, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return false, nil
	}
	return err == nil, err
}

// CurrentBranch is the checked-out branch, or "" when detached.
func (r *Repo) CurrentBranch(ctx context.Context) (string, error) {
	out, err := run(ctx, r.Root, "symbolic-ref", "--quiet", "--short", "HEAD")
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return "", nil
	}
	return strings.TrimSpace(out), err
}

// CreateBranch creates name from HEAD and switches to it.
func (r *Repo) CreateBranch(ctx context.Context, name string) error {
	_, err := run(ctx, r.Root, "switch", "--quiet", "--create", name)
	return err
}

// SwitchBranch switches to an existing branch.
func (r *Repo) SwitchBranch(ctx context.Context, name string) error {
	_, err := run(ctx, r.Root, "switch", "--quiet", name)
	return err
}

// HasRemoteFor reports whether a remote points at repository
// ("owner/name", compared without case).
func (r *Repo) HasRemoteFor(ctx context.Context, repository string) (bool, error) {
	out, err := run(ctx, r.Root, "remote", "-v")
	if err != nil {
		return false, err
	}
	for line := range strings.Lines(out) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(repoName(fields[1]), repository) {
			return true, nil
		}
	}
	return false, nil
}

// repoName is "owner/name" from a remote URL, scp-like address or path.
func repoName(remote string) string {
	s := strings.TrimSuffix(strings.TrimSuffix(remote, "/"), ".git")
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	} else if at := strings.Index(s, ":"); at >= 0 && !strings.HasPrefix(s, "/") {
		s = s[at+1:] // git@host:owner/name
	}
	parts := strings.Split(strings.Trim(s, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	// Arguments are trk's own; branch names are checked by the caller.
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // See above.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			ee.Stderr = stderr.Bytes()
			return string(out), err
		}
		return "", fmt.Errorf("running git: %w", err)
	}
	return string(out), nil
}
