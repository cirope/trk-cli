package api

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// AttachmentPage is one page of a card's files.
type AttachmentPage struct {
	Items         []Attachment `json:"items"`
	NextPageToken *string      `json:"next_page_token"`
}

// MessagePage is one page of a card's conversation, oldest first; each
// next page is older.
type MessagePage struct {
	Items         []Message `json:"items"`
	NextPageToken *string   `json:"next_page_token"`
}

// CardAttachments returns the page of a card's files after pageToken.
func (c *Client) CardAttachments(ctx context.Context, id, pageToken string) (*AttachmentPage, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out AttachmentPage
	q := url.Values{"page_token": {pageToken}}
	if err := c.Do(ctx, http.MethodGet, "/cards/"+id+"/attachments", q, nil, &out); err != nil {
		return nil, fmt.Errorf("listing the files of card %s: %w", id, err)
	}
	return &out, nil
}

// CardMessages returns the page of a card's conversation before
// pageToken.
func (c *Client) CardMessages(ctx context.Context, id, pageToken string) (*MessagePage, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out MessagePage
	q := url.Values{"page_token": {pageToken}}
	if err := c.Do(ctx, http.MethodGet, "/cards/"+id+"/messages", q, nil, &out); err != nil {
		return nil, fmt.Errorf("listing the messages of card %s: %w", id, err)
	}
	return &out, nil
}

// ErrSizeMismatch means a download was not the size the API said.
var ErrSizeMismatch = errors.New("the file is not the size the server said")

// Download streams attachment id to w and checks it is exactly size
// bytes; it never writes more than size. It is not retried.
func (c *Client) Download(ctx context.Context, id string, size int64, w io.Writer) (int64, error) {
	if err := checkID(id); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.o.DownloadTimeout)
	defer cancel()

	u, err := c.url("/attachments/" + id)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "*/*")
	c.authorize(req)

	resp, err := c.o.HTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("downloading file %s: %w", id, err)
	}
	defer func() { _ = resp.Body.Close() }()
	c.o.Logger.Debug("request", "method", http.MethodGet, "path", u.Path, "status", resp.StatusCode)
	if resp.StatusCode >= 300 {
		data, _ := c.read(resp.Body)
		return 0, fmt.Errorf("downloading file %s: %w", id, c.parseError(resp.StatusCode, resp.Status, data))
	}

	n, err := io.Copy(w, io.LimitReader(resp.Body, size))
	if err != nil {
		return n, fmt.Errorf("downloading file %s: %w", id, err)
	}
	extra, _ := resp.Body.Read(make([]byte, 1))
	if n != size || extra > 0 {
		return n, fmt.Errorf("downloading file %s: %w", id, ErrSizeMismatch)
	}
	return n, nil
}

// NewCard is a card to create. Board (an id or a name) and Title are
// required; empty options are left out, so the server's defaults apply.
type NewCard struct {
	Board       string `json:"board"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Priority    string `json:"priority,omitempty"`
	DueOn       string `json:"due_on,omitempty"` // YYYY-MM-DD
	Column      string `json:"column,omitempty"` // A name or a role; default the board's To do.
}

// CreateCard creates a card. It never starts Trk's agent.
func (c *Client) CreateCard(ctx context.Context, n NewCard) (*Card, error) {
	var out Card
	if err := c.Do(ctx, http.MethodPost, "/cards", nil, n, &out); err != nil {
		return nil, fmt.Errorf("creating the card: %w", err)
	}
	return &out, nil
}

// MoveCard moves card id to a column, by name or role. Into the done
// column it closes the card and merges its open pull request; close
// confirms that, and the server refuses such a move without it (409
// close_required).
func (c *Client) MoveCard(ctx context.Context, id, column string, close bool) (*Card, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	body := struct {
		Column string `json:"column"`
		Close  bool   `json:"close,omitempty"`
	}{column, close}
	var out Card
	if err := c.Do(ctx, http.MethodPost, "/cards/"+id+"/move", nil, body, &out); err != nil {
		return nil, fmt.Errorf("moving card %s: %w", id, err)
	}
	return &out, nil
}

// Posted is a message posted on a card's conversation. AgentTurn means
// Trk's agent answers it with a new turn.
type Posted struct {
	Message     Message      `json:"message"`
	AgentTurn   bool         `json:"agent_turn"`
	Attachments []Attachment `json:"attachments"` // The files sent with it.
}

// PostMessage posts body (Markdown) on card id's conversation, as a reply
// in the web app does: the card goes back to In progress.
// With files, it sends them with the message; body may then be empty.
func (c *Client) PostMessage(ctx context.Context, id, body string, files ...Upload) (*Posted, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out Posted
	var err error
	if len(files) > 0 {
		var fields []field
		if body != "" {
			fields = append(fields, field{"body", body})
		}
		err = c.sendMultipart(ctx, "/cards/"+id+"/messages", fields, files, &out)
	} else {
		err = c.Do(ctx, http.MethodPost, "/cards/"+id+"/messages", nil, map[string]string{"body": body}, &out)
	}
	if err != nil {
		return nil, fmt.Errorf("posting on card %s: %w", id, err)
	}
	return &out, nil
}

// AddOwner makes person ("me", an email or a person id) an owner of card
// id. Someone who already owns it changes nothing.
func (c *Client) AddOwner(ctx context.Context, id, person string) (*Card, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out Card
	body := map[string]string{"person": person}
	if err := c.Do(ctx, http.MethodPost, "/cards/"+id+"/owners", nil, body, &out); err != nil {
		return nil, fmt.Errorf("adding %s as an owner of card %s: %w", person, id, err)
	}
	return &out, nil
}

// RemoveOwner takes person ("me", an email or a person id) off card id's
// owners. Someone who does not own it changes nothing.
func (c *Client) RemoveOwner(ctx context.Context, id, person string) (*Card, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out Card
	if err := c.Do(ctx, http.MethodDelete, "/cards/"+id+"/owners/"+url.PathEscape(person), nil, nil, &out); err != nil {
		return nil, fmt.Errorf("removing %s from the owners of card %s: %w", person, id, err)
	}
	return &out, nil
}

// CardUpdate is a change to a card: only the fields that are not nil are
// sent. An empty DueOn clears the due date.
type CardUpdate struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Type        *string `json:"type,omitempty"`
	Priority    *string `json:"priority,omitempty"`
	DueOn       *string `json:"due_on,omitempty"`
}

// UpdateCard changes card id.
func (c *Client) UpdateCard(ctx context.Context, id string, u CardUpdate) (*Card, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out Card
	if err := c.Do(ctx, http.MethodPatch, "/cards/"+id, nil, u, &out); err != nil {
		return nil, fmt.Errorf("changing card %s: %w", id, err)
	}
	return &out, nil
}

// LinkPullRequest links a pull request of the board's repository to card
// id, for work done on a branch other than the card's.
func (c *Client) LinkPullRequest(ctx context.Context, id, prURL string) (*Card, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out Card
	body := map[string]string{"url": prURL}
	if err := c.Do(ctx, http.MethodPost, "/cards/"+id+"/pull", nil, body, &out); err != nil {
		return nil, fmt.Errorf("linking %s to card %s: %w", prURL, id, err)
	}
	return &out, nil
}

// Rating is the team's verdict on Trk's agent's latest answer on a card:
// good or bad, with what was wrong (Note) and what was sent in the end
// (Sent). Empty texts are left out.
type Rating struct {
	Verdict string `json:"verdict"`
	Note    string `json:"note,omitempty"`
	Sent    string `json:"sent,omitempty"`
}

// Rated is a recorded rating and the tally of the turn it rates.
type Rated struct {
	Rating struct {
		Turn    string  `json:"turn"`
		Verdict string  `json:"verdict"`
		Note    *string `json:"note"`
		Sent    *string `json:"sent"`
	} `json:"rating"`
	Tally struct {
		Good int `json:"good"`
		Bad  int `json:"bad"`
	} `json:"tally"`
}

// RateAnswer rates Trk's agent's latest answer on card id. One rating per
// person per turn: rating again changes it. A bad one reaches the agent
// on its next turn on the card.
func (c *Client) RateAnswer(ctx context.Context, id string, r Rating) (*Rated, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out Rated
	if err := c.Do(ctx, http.MethodPost, "/cards/"+id+"/rating", nil, r, &out); err != nil {
		return nil, fmt.Errorf("rating the answer on card %s: %w", id, err)
	}
	return &out, nil
}
