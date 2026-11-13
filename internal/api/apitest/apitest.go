// Package apitest is a fake trk server for tests, built from recorded
// fixtures of the real API's answers (testdata/fixtures, one file per
// endpoint and case). The same files are checked against trk's real
// controllers, so the fake cannot drift from the server unnoticed.
package apitest

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"
)

//go:embed testdata/fixtures/*.json
var files embed.FS

// Fixture is one recorded request and its answer.
type Fixture struct {
	Name    string `json:"-"`
	Request struct {
		Method string            `json:"method"`
		Path   string            `json:"path"`
		Query  map[string]string `json:"query"`
		Body   json.RawMessage   `json:"body"`
	} `json:"request"`
	Response struct {
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers"`
		// Body is the JSON answer, or a JSON string holding the raw
		// answer when Content-Type is not JSON (Markdown, files).
		Body json.RawMessage `json:"body"`
	} `json:"response"`
}

// Names lists every fixture, sorted.
func Names() []string {
	entries, _ := files.ReadDir("testdata/fixtures")
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	return names
}

// Load reads the fixture called name ("card.get.ok").
func Load(t testing.TB, name string) Fixture {
	t.Helper()
	b, err := files.ReadFile(path.Join("testdata/fixtures", name+".json"))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	var f Fixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	f.Name = name
	return f
}

// Server answers requests with the fixtures it was given. When several
// match a request they are served in order, and the last one repeats
// (a device flow's pending, pending, approved). A request no fixture
// matches fails the test.
type Server struct {
	*httptest.Server
	t        testing.TB
	mu       sync.Mutex
	fixtures []Fixture
	served   map[int]bool
	requests []Request
}

// Request is a request the server received.
type Request struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
}

// NewServer starts a server answering with the named fixtures. It stops
// when the test ends.
func NewServer(t testing.TB, names ...string) *Server {
	t.Helper()
	s := &Server{t: t, served: map[int]bool{}}
	for _, n := range names {
		s.fixtures = append(s.fixtures, Load(t, n))
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Requests returns the requests received so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, Request{
		Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
		Header: r.Header.Clone(), Body: body,
	})
	f, ok := s.match(r, body)
	s.mu.Unlock()

	if !ok {
		s.t.Errorf("apitest: no fixture for %s %s", r.Method, r.URL.RequestURI())
		http.Error(w, "no fixture", http.StatusNotImplemented)
		return
	}
	for k, v := range f.Response.Headers {
		w.Header().Set(k, v)
	}
	out := []byte(f.Response.Body)
	if ct := w.Header().Get("Content-Type"); ct == "" {
		w.Header().Set("Content-Type", "application/json")
	} else if !strings.HasPrefix(ct, "application/json") {
		var raw string
		if err := json.Unmarshal(f.Response.Body, &raw); err != nil {
			s.t.Errorf("apitest: %s: a %s body must be a JSON string", f.Name, ct)
		}
		out = []byte(raw)
	}
	w.WriteHeader(f.Response.Status)
	if f.Response.Status != http.StatusNoContent {
		_, _ = w.Write(out)
	}
}

// match picks the first unserved matching fixture, else the last match.
func (s *Server) match(r *http.Request, body []byte) (Fixture, bool) {
	last := -1
	for i, f := range s.fixtures {
		if !matches(f, r, body) {
			continue
		}
		if !s.served[i] {
			s.served[i] = true
			return f, true
		}
		last = i
	}
	if last < 0 {
		return Fixture{}, false
	}
	return s.fixtures[last], true
}

func matches(f Fixture, r *http.Request, body []byte) bool {
	if f.Request.Method != r.Method || f.Request.Path != r.URL.Path {
		return false
	}
	q := r.URL.Query()
	if len(q) != len(f.Request.Query) {
		return false
	}
	for k, v := range f.Request.Query {
		if q.Get(k) != v {
			return false
		}
	}
	if len(f.Request.Body) == 0 {
		return true
	}
	return jsonEqual(f.Request.Body, body)
}

func jsonEqual(a, b []byte) bool {
	var x, y bytes.Buffer
	if json.Compact(&x, a) != nil || json.Compact(&y, b) != nil {
		return false
	}
	var xv, yv any
	_ = json.Unmarshal(x.Bytes(), &xv)
	_ = json.Unmarshal(y.Bytes(), &yv)
	xb, _ := json.Marshal(xv) // maps marshal with sorted keys
	yb, _ := json.Marshal(yv)
	return bytes.Equal(xb, yb)
}
