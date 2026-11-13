// Package api is the HTTP client for trk's /api/v1: one typed method per
// endpoint over a small core that handles headers, errors, timeouts and
// retries.
package api

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 3
	defaultMaxBody    = 10 << 20 // JSON answers; files are streamed separately.
	baseBackoff       = 250 * time.Millisecond
	maxRetryAfter     = 30 * time.Second
)

// Options configures a Client. Zero values pick sensible defaults.
type Options struct {
	Host       string // https://trk.hullop.com, possibly with a path prefix.
	Token      Token  // Empty for the device flow's first calls.
	UserAgent  string // trk-cli/<version> (<os>; <arch>)
	HTTPClient *http.Client
	Logger     *slog.Logger // Requests and timings at debug level. Never tokens.
	Timeout    time.Duration
	MaxRetries int   // Retries of idempotent requests; negative means none.
	MaxBody    int64 // Largest JSON answer read.

	// Sleep waits between retries; tests replace it to not sleep.
	Sleep func(context.Context, time.Duration) error
	// Jitter returns a number in [0, 1) that spreads retries out.
	Jitter func() float64
}

// Client talks to one trk install as one session.
type Client struct {
	base *url.URL
	o    Options
}

// New returns a client for opts.Host.
func New(opts Options) (*Client, error) {
	u, err := url.Parse(opts.Host)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("host %q is not an http(s) URL", opts.Host)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/api/v1"
	u.RawQuery, u.Fragment = "", ""

	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{}
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = defaultMaxRetries
	} else if opts.MaxRetries < 0 {
		opts.MaxRetries = 0
	}
	if opts.MaxBody <= 0 {
		opts.MaxBody = defaultMaxBody
	}
	if opts.Sleep == nil {
		opts.Sleep = sleep
	}
	if opts.Jitter == nil {
		opts.Jitter = rand.Float64
	}
	return &Client{base: u, o: opts}, nil
}

// Do sends a request to path (under /api/v1) with query and, when not
// nil, body as JSON, and decodes a successful JSON answer into out when
// out is not nil. Idempotent requests are retried on connection errors,
// 429 and 502/503/504.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("encoding the request: %w", err)
		}
	}
	data, err := c.send(ctx, method, path, query, payload, "application/json")
	if err != nil {
		return err
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("reading the answer to %s %s: %w", method, path, err)
	}
	return nil
}

// send runs the retry loop and returns a successful answer's body.
// accept is the media type asked for.
func (c *Client) send(ctx context.Context, method, path string, query url.Values, payload []byte, accept string) ([]byte, error) {
	retries := 0
	if idempotent(method) {
		retries = c.o.MaxRetries
	}
	for attempt := 0; ; attempt++ {
		data, wait, err := c.attempt(ctx, method, path, query, payload, accept)
		if err == nil {
			return data, nil
		}
		if wait < 0 || attempt >= retries || ctx.Err() != nil {
			return nil, err
		}
		if wait == 0 {
			wait = backoff(attempt, c.o.Jitter())
		}
		c.o.Logger.Debug("retrying", "method", method, "path", path, "after", wait, "error", err)
		if err := c.o.Sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

// attempt sends the request once. wait is negative when the failure is
// not worth retrying, zero to use the backoff, or what the server asked.
func (c *Client) attempt(ctx context.Context, method, path string, query url.Values, payload []byte, accept string) (data []byte, wait time.Duration, err error) {
	ctx, cancel := context.WithTimeout(ctx, c.o.Timeout)
	defer cancel()

	u := *c.base
	u.Path += path
	u.RawQuery = query.Encode()

	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, -1, fmt.Errorf("building %s %s: %w", method, path, err)
	}
	req.Header.Set("Accept", accept)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.o.UserAgent != "" {
		req.Header.Set("User-Agent", c.o.UserAgent)
	}
	if c.o.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.o.Token.Reveal())
	}

	start := time.Now()
	resp, err := c.o.HTTPClient.Do(req)
	if err != nil {
		c.o.Logger.Debug("request failed", "method", method, "path", path, "took", time.Since(start), "error", err)
		if ctx.Err() != nil { // a timeout or a cancel, not a dropped connection
			return nil, -1, fmt.Errorf("%s %s: %w", method, path, ctx.Err())
		}
		return nil, 0, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	c.o.Logger.Debug("request", "method", method, "path", path, "status", resp.StatusCode, "took", time.Since(start))

	data, err = c.read(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, -1, fmt.Errorf("reading the answer to %s %s: %w", method, path, err)
	}
	if resp.StatusCode < 300 {
		return data, 0, nil
	}

	apiErr := c.parseError(resp.StatusCode, resp.Status, data)
	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return nil, retryAfter(resp.Header.Get("Retry-After"), time.Now()), apiErr
	}
	return nil, -1, apiErr
}

// read reads a body up to MaxBody.
func (c *Client) read(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, c.o.MaxBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > c.o.MaxBody {
		return nil, ErrTooLarge
	}
	return data, nil
}

// parseError builds an *Error from an error answer, which may not be the
// envelope at all (a proxy's HTML page). The token never survives into
// the message.
func (c *Client) parseError(status int, statusLine string, data []byte) *Error {
	e := &Error{Status: status}
	var env struct {
		Error       json.RawMessage `json:"error"`
		Description string          `json:"error_description"`
	}
	var obj struct {
		Code    string              `json:"code"`
		Message string              `json:"message"`
		Fields  map[string][]string `json:"fields"`
	}
	switch {
	case json.Unmarshal(data, &env) != nil:
	case json.Unmarshal(env.Error, &obj) == nil && obj.Message != "":
		e.Code, e.Message, e.Fields = obj.Code, obj.Message, obj.Fields
	case json.Unmarshal(env.Error, &e.Code) == nil && e.Code != "":
		// The device flow's RFC 8628 errors: {"error": "slow_down"}.
		e.Message = cmp.Or(env.Description, e.Code)
	}
	if e.Message == "" {
		e.Code, e.Message = "", "the server answered "+statusLine
	}
	if tok := c.o.Token.Reveal(); tok != "" {
		e.Message = strings.ReplaceAll(e.Message, tok, redacted)
	}
	return e
}

func idempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

// backoff is the wait before retry number attempt (from 0), grown
// exponentially and spread by jitter in [0, 1).
func backoff(attempt int, jitter float64) time.Duration {
	d := baseBackoff << attempt
	return d + time.Duration(float64(d/2)*jitter)
}

// retryAfter parses a Retry-After header (seconds or an HTTP date),
// capped at maxRetryAfter. Zero means none was given.
func retryAfter(h string, now time.Time) time.Duration {
	if h == "" {
		return 0
	}
	var d time.Duration
	if s, err := strconv.Atoi(h); err == nil {
		d = time.Duration(s) * time.Second
	} else if t, err := http.ParseTime(h); err == nil {
		d = t.Sub(now)
	}
	return max(0, min(d, maxRetryAfter))
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
