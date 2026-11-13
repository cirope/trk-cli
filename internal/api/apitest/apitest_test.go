package apitest

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func get(t *testing.T, srv *Server, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestEveryFixtureIsWellFormed(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Fatal("no fixtures found")
	}
	for _, name := range names {
		f := Load(t, name)
		if f.Request.Method == "" || !strings.HasPrefix(f.Request.Path, "/api/v1/") {
			t.Errorf("%s: request needs a method and an /api/v1/ path, got %q %q", name, f.Request.Method, f.Request.Path)
		}
		if f.Response.Status < 100 {
			t.Errorf("%s: response needs a status", name)
		}
		if !json.Valid(f.Response.Body) {
			t.Errorf("%s: response body is not JSON", name)
		}
	}
}

func TestServesTheMatchingFixture(t *testing.T) {
	srv := NewServer(t, "me.ok")
	status, body := get(t, srv, "/api/v1/me")
	if status != http.StatusOK || !strings.Contains(body, `"organization"`) {
		t.Errorf("GET /api/v1/me = %d %s", status, body)
	}
	if got := len(srv.Requests()); got != 1 {
		t.Errorf("recorded %d requests, want 1", got)
	}
}

func TestServesSequenceThenRepeatsTheLast(t *testing.T) {
	srv := NewServer(t, "card.get.not_found", "card.get.ok")
	var got []int
	for range 3 {
		status, _ := get(t, srv, "/api/v1/cards/BaHtsbhzVDa9")
		got = append(got, status)
	}
	want := []int{404, 200, 200}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("statuses = %v, want %v", got, want)
		}
	}
}

func TestTextBodiesAreWrittenRaw(t *testing.T) {
	srv := NewServer(t, "card.markdown.ok")
	status, body := get(t, srv, "/api/v1/cards/BaHtsbhzVDa9/markdown?files=linked")
	if status != http.StatusOK || !strings.HasPrefix(body, "# CSV export loses accents") {
		t.Errorf("markdown = %d %q", status, body)
	}
}

func TestUnmatchedRequestFailsTheTest(t *testing.T) {
	ft := &fakeT{TB: t}
	srv := NewServer(ft, "me.ok")
	status, _ := get(t, srv, "/api/v1/cards")
	if status != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", status)
	}
	if !ft.failed {
		t.Error("an unmatched request did not fail the test")
	}
}

type fakeT struct {
	testing.TB
	failed bool
}

func (f *fakeT) Errorf(string, ...any) { f.failed = true }
