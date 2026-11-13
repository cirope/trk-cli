package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// newTestClient returns a client for srv that records its backoff sleeps
// instead of sleeping.
func newTestClient(t *testing.T, srv *httptest.Server, opts ...func(*Options)) (*Client, *[]time.Duration) {
	t.Helper()
	var slept []time.Duration
	o := Options{
		Host:       srv.URL,
		Token:      Token(secret),
		UserAgent:  "trk-cli/1.2.3 (darwin; arm64)",
		HTTPClient: srv.Client(),
		Timeout:    2 * time.Second,
		MaxRetries: 2,
		Sleep: func(ctx context.Context, d time.Duration) error {
			slept = append(slept, d)
			return ctx.Err()
		},
		Jitter: func() float64 { return 0.5 },
	}
	for _, f := range opts {
		f(&o)
	}
	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return c, &slept
}

func TestRequestShape(t *testing.T) {
	var got *http.Request
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"Ana","unknown_field_from_newer_server":true}`)
	}))
	defer srv.Close()
	c, _ := newTestClient(t, srv)

	var out struct{ Name string }
	err := c.Do(context.Background(), http.MethodPost, "/cards/BaHtsbhzVDa9/take", nil, map[string]string{"a": "b"}, &out)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if out.Name != "Ana" {
		t.Errorf("decoded name = %q, want Ana", out.Name)
	}
	checks := []struct{ what, got, want string }{
		{"method", got.Method, http.MethodPost},
		{"path", got.URL.Path, "/api/v1/cards/BaHtsbhzVDa9/take"},
		{"authorization", got.Header.Get("Authorization"), "Bearer " + secret},
		{"user agent", got.Header.Get("User-Agent"), "trk-cli/1.2.3 (darwin; arm64)"},
		{"accept", got.Header.Get("Accept"), "application/json"},
		{"content type", got.Header.Get("Content-Type"), "application/json"},
		{"body", strings.TrimSpace(string(body)), `{"a":"b"}`},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.what, c.got, c.want)
		}
	}
}

func TestQueryAndNoToken(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	c, _ := newTestClient(t, srv, func(o *Options) { o.Token = "" })

	q := map[string][]string{"limit": {"30"}, "state": {"open"}}
	if err := c.Do(context.Background(), http.MethodGet, "/cards", q, nil, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got.URL.RawQuery != "limit=30&state=open" {
		t.Errorf("query = %q", got.URL.RawQuery)
	}
	if h := got.Header.Get("Authorization"); h != "" {
		t.Errorf("Authorization = %q, want none without a token", h)
	}
}

func TestHostWithPath(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	c, _ := newTestClient(t, srv, func(o *Options) { o.Host = srv.URL + "/trk/" })
	if err := c.Do(context.Background(), http.MethodGet, "/me", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if path != "/trk/api/v1/me" {
		t.Errorf("path = %q, want /trk/api/v1/me", path)
	}
}

func TestNewRejectsBadHosts(t *testing.T) {
	for _, host := range []string{"", "trk.hullop.com", "ftp://trk.hullop.com", "http://"} {
		if _, err := New(Options{Host: host}); err == nil {
			t.Errorf("New(Host: %q) succeeded, want an error", host)
		}
	}
}

func TestErrorStatuses(t *testing.T) {
	tests := []struct {
		status   int
		body     string
		sentinel error
		want     Error
	}{
		{400, `{"error":{"code":"bad_request","message":"Bad limit."}}`, nil,
			Error{Status: 400, Code: "bad_request", Message: "Bad limit."}},
		{401, `{"error":{"code":"unauthorized","message":"Sign in again."}}`, ErrUnauthorized,
			Error{Status: 401, Code: "unauthorized", Message: "Sign in again."}},
		{403, `{"error":{"code":"forbidden","message":"Not allowed."}}`, ErrForbidden,
			Error{Status: 403, Code: "forbidden", Message: "Not allowed."}},
		{404, `{"error":{"code":"not_found","message":"No such card."}}`, ErrNotFound,
			Error{Status: 404, Code: "not_found", Message: "No such card."}},
		{409, `{"error":{"code":"conflict","message":"Agent is working."}}`, ErrConflict,
			Error{Status: 409, Code: "conflict", Message: "Agent is working."}},
		{422, `{"error":{"code":"validation_failed","message":"Invalid.","fields":{"title":["can't be blank"]}}}`, nil,
			Error{Status: 422, Code: "validation_failed", Message: "Invalid.", Fields: map[string][]string{"title": {"can't be blank"}}}},
		{426, `{"error":{"code":"upgrade_required","message":"Update trk to 1.4 or later."}}`, ErrUpgradeRequired,
			Error{Status: 426, Code: "upgrade_required", Message: "Update trk to 1.4 or later."}},
		{500, `{"error":{"code":"server_error","message":"Oops."}}`, nil,
			Error{Status: 500, Code: "server_error", Message: "Oops."}},
		{502, `<html>Bad gateway</html>`, nil,
			Error{Status: 502, Message: "the server answered 502 Bad Gateway"}},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			c, _ := newTestClient(t, srv, func(o *Options) { o.MaxRetries = 0 })

			err := c.Do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v (%T), want *Error", err, err)
			}
			if diff := cmp.Diff(tt.want, *apiErr); diff != "" {
				t.Errorf("error mismatch (-want +got):\n%s", diff)
			}
			if tt.sentinel != nil && !errors.Is(err, tt.sentinel) {
				t.Errorf("errors.Is(%v, %v) = false", err, tt.sentinel)
			}
		})
	}
}

func TestErrorMessageNeverCarriesTheToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"unauthorized","message":"bad token `+secret+`"}}`)
	}))
	defer srv.Close()
	c, _ := newTestClient(t, srv)

	err := c.Do(context.Background(), http.MethodGet, "/me", nil, nil, nil)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Errorf("err = %v, want an error without the token", err)
	}
}

func TestMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"name": `)
	}))
	defer srv.Close()
	c, _ := newTestClient(t, srv)

	var out struct{ Name string }
	err := c.Do(context.Background(), http.MethodGet, "/me", nil, nil, &out)
	if err == nil || !strings.Contains(err.Error(), "reading the answer") {
		t.Errorf("err = %v, want a decoding error", err)
	}
}

func TestEnormousBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"name":"`)
		chunk := strings.Repeat("a", 64<<10)
		for range 64 { // 4 MiB, over the 1 MiB test limit
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
		}
		_, _ = io.WriteString(w, `"}`)
	}))
	defer srv.Close()
	c, _ := newTestClient(t, srv, func(o *Options) { o.MaxBody = 1 << 20 })

	var out struct{ Name string }
	err := c.Do(context.Background(), http.MethodGet, "/me", nil, nil, &out)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // never answers
	}))
	defer srv.Close()
	c, _ := newTestClient(t, srv, func(o *Options) {
		o.Timeout = 50 * time.Millisecond
		o.MaxRetries = 0
	})

	start := time.Now()
	err := c.Do(context.Background(), http.MethodGet, "/me", nil, nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want a deadline error", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("took %v, want the 50ms timeout to stop it", d)
	}
}

func TestCancelStopsPromptly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	c, _ := newTestClient(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	err := c.Do(ctx, http.MethodGet, "/me", nil, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestRetries(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		answers   []int // status per attempt; 0 drops the connection
		header    string
		wantCalls int32
		wantErr   bool
		wantSlept []time.Duration
	}{
		{"GET retried on 503", http.MethodGet, []int{503, 200}, "", 2, false,
			[]time.Duration{backoff(0, 0.5)}},
		{"GET retried on 502 and 504", http.MethodGet, []int{502, 504, 200}, "", 3, false,
			[]time.Duration{backoff(0, 0.5), backoff(1, 0.5)}},
		{"GET retried on dropped connection", http.MethodGet, []int{0, 200}, "", 2, false,
			[]time.Duration{backoff(0, 0.5)}},
		{"GET gives up after MaxRetries", http.MethodGet, []int{503, 503, 503, 200}, "", 3, true,
			[]time.Duration{backoff(0, 0.5), backoff(1, 0.5)}},
		{"Retry-After honoured", http.MethodGet, []int{503, 200}, "3", 2, false,
			[]time.Duration{3 * time.Second}},
		{"429 retried with Retry-After", http.MethodGet, []int{429, 200}, "1", 2, false,
			[]time.Duration{time.Second}},
		{"DELETE is idempotent", http.MethodDelete, []int{503, 204}, "", 2, false,
			[]time.Duration{backoff(0, 0.5)}},
		{"POST never retried", http.MethodPost, []int{503, 200}, "", 1, true, nil},
		{"POST not retried on dropped connection", http.MethodPost, []int{0, 200}, "", 1, true, nil},
		{"500 not retried", http.MethodGet, []int{500, 200}, "", 1, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				n := calls.Add(1)
				status := tt.answers[n-1]
				if status == 0 {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						_ = conn.Close()
					}
					return
				}
				if tt.header != "" {
					w.Header().Set("Retry-After", tt.header)
				}
				w.WriteHeader(status)
				if status < 300 && status != 204 {
					_, _ = io.WriteString(w, `{}`)
				}
			}))
			defer srv.Close()
			c, slept := newTestClient(t, srv)

			err := c.Do(context.Background(), tt.method, "/x", nil, nil, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got := calls.Load(); got != tt.wantCalls {
				t.Errorf("calls = %d, want %d", got, tt.wantCalls)
			}
			if diff := cmp.Diff(tt.wantSlept, *slept); diff != "" {
				t.Errorf("sleeps (-want +got):\n%s", diff)
			}
		})
	}
}

// Even with the most jitter, a later attempt waits longer than an earlier
// one with the least.
func TestBackoffGrowsDespiteJitter(t *testing.T) {
	for attempt := range 4 {
		most, next := backoff(attempt, 1), backoff(attempt+1, 0)
		if next <= most {
			t.Errorf("backoff(%d, 0) = %v, want more than backoff(%d, 1) = %v",
				attempt+1, next, attempt, most)
		}
	}
}

func TestRetryAfterIsCapped(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "86400")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	c, slept := newTestClient(t, srv)
	if err := c.Do(context.Background(), http.MethodGet, "/x", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 1 || (*slept)[0] > maxRetryAfter {
		t.Errorf("slept %v, want one sleep of at most %v", *slept, maxRetryAfter)
	}
}
