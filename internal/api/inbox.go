package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Area is a part of a board's work and who is in charge of it. People
// are ids in answers; in requests, "me", an email or an id.
type Area struct {
	Name   string   `json:"name"`
	Covers string   `json:"covers"`
	People []string `json:"people"`
}

// InboxItem is a card in the person's inbox, and why it is there.
type InboxItem struct {
	Card struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Priority string   `json:"priority"`
		Area     *string  `json:"area"`
		Board    BoardRef `json:"board"`
		URL      string   `json:"url"`
	} `json:"card"`
	State        string     `json:"state"` // waiting, snoozed, done
	SnoozedUntil *time.Time `json:"snoozed_until"`
	TaggedAt     time.Time  `json:"tagged_at"`
	Seen         bool       `json:"seen"`
	Reason       struct {
		Why  []string `json:"why"` // router, priority, area, customer, review, question, person
		Area *string  `json:"area"`
		By   *string  `json:"by"`
	} `json:"reason"`
}

// InboxPage is one page of the inbox.
type InboxPage struct {
	Items         []InboxItem `json:"items"`
	NextPageToken *string     `json:"next_page_token"`
}

// Inbox lists the person's inbox: state "" (waiting), snoozed or done.
func (c *Client) Inbox(ctx context.Context, state, pageToken string) (*InboxPage, error) {
	q := url.Values{}
	if state != "" {
		q.Set("state", state)
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	var out InboxPage
	if err := c.Do(ctx, http.MethodGet, "/inbox", q, nil, &out); err != nil {
		return nil, fmt.Errorf("listing your inbox: %w", err)
	}
	return &out, nil
}

// InboxDone marks card id done in the person's inbox.
func (c *Client) InboxDone(ctx context.Context, id string) (*InboxItem, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out InboxItem
	if err := c.Do(ctx, http.MethodPost, "/inbox/"+id+"/done", nil, nil, &out); err != nil {
		return nil, fmt.Errorf("marking card %s done in your inbox: %w", id, err)
	}
	return &out, nil
}

// InboxSnooze snoozes card id in the person's inbox until later_today,
// tomorrow, next_week, or a future ISO 8601 date or date-time.
func (c *Client) InboxSnooze(ctx context.Context, id, until string) (*InboxItem, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out InboxItem
	if err := c.Do(ctx, http.MethodPost, "/inbox/"+id+"/snooze", nil, map[string]string{"until": until}, &out); err != nil {
		return nil, fmt.Errorf("snoozing card %s: %w", id, err)
	}
	return &out, nil
}
