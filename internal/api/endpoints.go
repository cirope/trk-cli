package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
)

// ErrBadID is returned, without asking the server, for an id that is not
// a trk id.
var ErrBadID = errors.New("not a trk id")

// trk ids are 12 characters from a 57-symbol alphabet: 2-9, A-Z without
// I and O, a-z without l.
var idPattern = regexp.MustCompile(`^[2-9A-HJ-NP-Za-km-z]{12}$`)

// ValidID reports whether id has the shape of a trk id.
func ValidID(id string) bool { return idPattern.MatchString(id) }

func checkID(id string) error {
	if !ValidID(id) {
		return fmt.Errorf("%q: %w", id, ErrBadID)
	}
	return nil
}

// DeviceCode starts signing in. clientName names this machine on the
// approval page and in the person's sessions.
func (c *Client) DeviceCode(ctx context.Context, clientName string) (*DeviceCode, error) {
	var out DeviceCode
	body := map[string]string{"client_name": clientName}
	if err := c.Do(ctx, http.MethodPost, "/device/code", nil, body, &out); err != nil {
		return nil, fmt.Errorf("starting to sign in: %w", err)
	}
	return &out, nil
}

// DeviceToken polls for the token. While waiting it fails with an *Error
// whose Code is authorization_pending or slow_down; access_denied and
// expired_token end the flow.
func (c *Client) DeviceToken(ctx context.Context, deviceCode string) (*TokenGrant, error) {
	var out TokenGrant
	body := map[string]string{"device_code": deviceCode}
	if err := c.Do(ctx, http.MethodPost, "/device/token", nil, body, &out); err != nil {
		return nil, err // Wrapping would only bury the code the caller polls on.
	}
	return &out, nil
}

// Me returns who the session belongs to.
func (c *Client) Me(ctx context.Context) (*Me, error) {
	var out Me
	if err := c.Do(ctx, http.MethodGet, "/me", nil, nil, &out); err != nil {
		return nil, fmt.Errorf("asking who you are: %w", err)
	}
	return &out, nil
}

// RevokeSession revokes the token in use.
func (c *Client) RevokeSession(ctx context.Context) error {
	if err := c.Do(ctx, http.MethodDelete, "/session", nil, nil, nil); err != nil {
		return fmt.Errorf("revoking the session: %w", err)
	}
	return nil
}

// CardsQuery filters and pages GET /cards. Zero values are left out.
type CardsQuery struct {
	Board     string
	State     string
	Type      string
	Priority  string
	All       bool
	Limit     int
	PageToken string
}

func (q CardsQuery) values() url.Values {
	v := url.Values{}
	set := func(k, val string) {
		if val != "" {
			v.Set(k, val)
		}
	}
	set("board", q.Board)
	set("state", q.State)
	set("type", q.Type)
	set("priority", q.Priority)
	set("page_token", q.PageToken)
	if q.All {
		v.Set("all", "true")
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	return v
}

// Cards lists cards: by default my open cards, across every board.
func (c *Client) Cards(ctx context.Context, q CardsQuery) (*CardPage, error) {
	var out CardPage
	if err := c.Do(ctx, http.MethodGet, "/cards", q.values(), nil, &out); err != nil {
		return nil, fmt.Errorf("listing cards: %w", err)
	}
	return &out, nil
}

// Card returns one card.
func (c *Client) Card(ctx context.Context, id string) (*Card, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out Card
	if err := c.Do(ctx, http.MethodGet, "/cards/"+id, nil, nil, &out); err != nil {
		return nil, fmt.Errorf("getting card %s: %w", id, err)
	}
	return &out, nil
}

// CardMarkdown returns the card as the server's Markdown, linking its
// files as attachments/<name>.
func (c *Client) CardMarkdown(ctx context.Context, id string) (string, error) {
	if err := checkID(id); err != nil {
		return "", err
	}
	q := url.Values{"files": {"linked"}}
	data, err := c.send(ctx, http.MethodGet, "/cards/"+id+"/markdown", q, nil, "text/markdown")
	if err != nil {
		return "", fmt.Errorf("getting card %s as Markdown: %w", id, err)
	}
	return string(data), nil
}

// Take sets a card up for work outside trk: owner, agent off, then moved
// to In progress, in that order, on the server.
func (c *Client) Take(ctx context.Context, id string) (*TakeResult, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out TakeResult
	if err := c.Do(ctx, http.MethodPost, "/cards/"+id+"/take", nil, nil, &out); err != nil {
		return nil, fmt.Errorf("taking card %s: %w", id, err)
	}
	return &out, nil
}
