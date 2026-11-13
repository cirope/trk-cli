// Package update tells people, at most once a day, when a newer trk has
// been released. It never updates anything itself.
package update

import (
	"context"
	"encoding/json"
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
	timeout  = 2 * time.Second
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
	if _, ok := parse(c.Current); !ok {
		return "" // A development build.
	}
	now := c.Now()
	st := c.load()
	if now.Sub(st.CheckedAt) >= interval {
		st.CheckedAt = now // Also on failure: try again tomorrow.
		if latest, err := c.fetch(ctx); err == nil {
			st.Latest = latest
		}
	}
	notice := ""
	if newer(st.Latest, c.Current) && now.Sub(st.NotifiedAt) >= interval {
		st.NotifiedAt = now
		notice = fmt.Sprintf("A new version of trk is available: %s (you have %s).",
			strings.TrimPrefix(st.Latest, "v"), c.Current)
	}
	c.save(st)
	return notice
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
