package update

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func server(t *testing.T, tag string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"tag_name":"`+tag+`","html_url":"https://example.test/r"}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func checker(t *testing.T, srv *httptest.Server, current string, now *time.Time) *Checker {
	t.Helper()
	return &Checker{
		Current:   current,
		StatePath: filepath.Join(t.TempDir(), "trk", "update.json"),
		URL:       srv.URL,
		HTTP:      srv.Client(),
		Now:       func() time.Time { return *now },
	}
}

func TestNoticeAtMostOnceADay(t *testing.T) {
	srv, calls := server(t, "v1.3.0")
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	c := checker(t, srv, "1.2.0", &now)

	n := c.Notice(context.Background())
	if !strings.Contains(n, "1.3.0") || !strings.Contains(n, "1.2.0") {
		t.Errorf("notice = %q, want both versions", n)
	}
	now = now.Add(time.Hour)
	if n := c.Notice(context.Background()); n != "" {
		t.Errorf("second notice the same day = %q", n)
	}
	if calls.Load() != 1 {
		t.Errorf("asked %d times in a day, want 1", calls.Load())
	}
	now = now.Add(24 * time.Hour)
	if n := c.Notice(context.Background()); n == "" {
		t.Error("no notice the next day")
	}
	if calls.Load() != 2 {
		t.Errorf("asked %d times over two days, want 2", calls.Load())
	}
}

func TestNoNoticeWhenCurrentOrNewer(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ current, latest string }{
		{"1.3.0", "v1.3.0"}, {"1.4.0", "v1.3.0"}, {"1.10.0", "v1.9.9"},
		{"dev", "v1.3.0"}, {"0.0.0-SNAPSHOT-8ab6322", "v1.3.0"}, {"1.2.0", "v2.0.0-rc.1"}, {"1.2.0", "garbage"},
	} {
		srv, _ := server(t, tc.latest)
		if n := checker(t, srv, tc.current, &now).Notice(context.Background()); n != "" {
			t.Errorf("current %s, latest %s: notice %q", tc.current, tc.latest, n)
		}
	}
}

func TestUnreachableIsSilent(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	c := &Checker{
		Current:   "1.2.0",
		StatePath: filepath.Join(t.TempDir(), "update.json"),
		URL:       "http://127.0.0.1:1",
		HTTP:      &http.Client{},
		Now:       func() time.Time { return now },
	}
	if n := c.Notice(context.Background()); n != "" {
		t.Errorf("notice = %q", n)
	}
}

func TestNewer(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"v1.3.0", "1.2.9", true},
		{"1.2.10", "1.2.9", true},
		{"2.0.0", "1.99.99", true},
		{"1.2.0", "1.2.0", false},
		{"1.2", "1.1.0", false}, // Not semver: never newer.
		{"v1.3.0-rc.1", "1.2.0", false},
	}
	for _, tt := range tests {
		if got := newer(tt.a, tt.b); got != tt.want {
			t.Errorf("newer(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
