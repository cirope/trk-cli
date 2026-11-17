// Package update tells people, at most once a day, when a newer trk has
// been released. It never updates anything itself.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// LatestURL is where the latest release is asked for.
const LatestURL = "https://api.github.com/repos/cirope/trk-cli/releases/latest"

const (
	interval = 24 * time.Hour
	timeout  = 1 * time.Second // Once a day at most; a slow answer is no answer.
)

// Checker checks for a newer release than Current.
type Checker struct {
	Current   string // This binary's version, e.g. 1.2.0.
	StatePath string // Where the last check is remembered.
	URL       string
	HTTP      *http.Client
	Now       func() time.Time
}

type state struct {
	CheckedAt  time.Time `json:"checked_at"`
	Latest     string    `json:"latest"`
	NotifiedAt time.Time `json:"notified_at"`
}

// Notice returns a line to show when a newer release exists, or "". It
// asks the network at most once a day, says so at most once a day, and
// is silent on any failure.
func (c *Checker) Notice(ctx context.Context) string {
	st, line := c.check(ctx)
	if line == "" || c.Now().Sub(st.NotifiedAt) < interval {
		c.save(st)
		return ""
	}
	st.NotifiedAt = c.Now()
	c.save(st)
	return line
}

// Newer is for every time it is asked, as when a command fails in a way
// a newer trk may fix: the newer release's version (no "v"), or "". It
// still asks the network at most once a day.
func (c *Checker) Newer(ctx context.Context) string {
	st, line := c.check(ctx)
	c.save(st)
	if line == "" {
		return ""
	}
	return strings.TrimPrefix(st.Latest, "v")
}

// Latest asks for the latest release now, whatever the day's check said,
// and remembers it for the notice. It returns the version without a "v";
// ErrDevelopment for a development build, which is not checked.
func (c *Checker) Latest(ctx context.Context) (string, error) {
	if _, ok := parse(c.Current); !ok {
		return "", ErrDevelopment
	}
	latest, err := c.fetch(ctx)
	if err != nil {
		return "", err
	}
	if _, ok := parse(latest); !ok {
		return "", fmt.Errorf("the latest release is %q, not a version", latest)
	}
	st := c.load()
	st.CheckedAt, st.Latest = c.Now(), latest
	c.save(st)
	return strings.TrimPrefix(latest, "v"), nil
}

// Outdated reports whether latest is newer than this build.
func (c *Checker) Outdated(latest string) bool { return newer(latest, c.Current) }

// ErrDevelopment is Latest's answer for a development build.
var ErrDevelopment = errors.New("a development build is not checked")

// check refreshes the remembered latest release when a day has passed
// and says whether it is newer than this one.
func (c *Checker) check(ctx context.Context) (state, string) {
	if _, ok := parse(c.Current); !ok {
		return c.load(), "" // A development build.
	}
	now := c.Now()
	st := c.load()
	if now.Sub(st.CheckedAt) >= interval {
		st.CheckedAt = now // Also on failure: try again tomorrow.
		if latest, err := c.fetch(ctx); err == nil {
			st.Latest = latest
		}
	}
	if !newer(st.Latest, c.Current) {
		return st, ""
	}
	return st, fmt.Sprintf("trk %s is out (you have %s)", strings.TrimPrefix(st.Latest, "v"), c.Current)
}

func (c *Checker) fetch(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return "", err
	}
	return rel.TagName, nil
}

func (c *Checker) load() state {
	var st state
	if b, err := os.ReadFile(c.StatePath); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

func (c *Checker) save(st state) {
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(c.StatePath), 0o700) == nil {
		_ = os.WriteFile(c.StatePath, b, 0o600)
	}
}

// newer reports whether version a is a release newer than b. Anything
// that is not a plain x.y.z release (a pre-release, garbage) is never
// newer.
func newer(a, b string) bool {
	va, ok1 := parse(a)
	vb, ok2 := parse(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
