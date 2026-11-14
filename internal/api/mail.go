package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Address is an email sender.
type Address struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// CardEmail says a card came by email.
type CardEmail struct {
	From     []string `json:"from"`
	Subject  string   `json:"subject"`
	Language *string  `json:"language"`
}

// MailThread is email Trk's router held instead of filing it as a card.
type MailThread struct {
	ID          string     `json:"id"`
	Subject     string     `json:"subject"`
	Status      string     `json:"status"`      // held, discarded, filed
	Reason      string     `json:"reason"`      // spam, phishing, marketing, automated, no_board, unsure, blocked, card_deleted
	ReasonText  *string    `json:"reason_text"` // The router's words, when it gave some.
	From        Address    `json:"from"`
	Snippet     string     `json:"snippet"`
	Emails      int        `json:"emails"`
	CardID      *string    `json:"card_id"`
	HeldAt      time.Time  `json:"held_at"`
	DiscardedAt *time.Time `json:"discarded_at"`
}

// MailPage is one page of held mail.
type MailPage struct {
	Items         []MailThread `json:"items"`
	NextPageToken *string      `json:"next_page_token"`
}

// MailQuery pages GET /mail; Discarded lists the discarded instead of
// the held.
type MailQuery struct {
	Discarded bool
	Limit     int
	PageToken string
}

func (q MailQuery) values() url.Values {
	v := url.Values{}
	if q.Discarded {
		v.Set("status", "discarded")
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.PageToken != "" {
		v.Set("page_token", q.PageToken)
	}
	return v
}

// Mail lists the email Trk's router held.
func (c *Client) Mail(ctx context.Context, q MailQuery) (*MailPage, error) {
	var out MailPage
	if err := c.Do(ctx, http.MethodGet, "/mail", q.values(), nil, &out); err != nil {
		return nil, fmt.Errorf("listing held mail: %w", err)
	}
	return &out, nil
}

// FileMail files a held thread as a card on board (an id or a name).
func (c *Client) FileMail(ctx context.Context, id, board string) (*Card, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out Card
	body := map[string]string{"board": board}
	if err := c.Do(ctx, http.MethodPost, "/mail/"+id+"/file", nil, body, &out); err != nil {
		return nil, fmt.Errorf("filing mail %s: %w", id, err)
	}
	return &out, nil
}

// DiscardMail discards a held thread; block also blocks its sender.
func (c *Client) DiscardMail(ctx context.Context, id string, block bool) (*MailThread, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	body := struct {
		Block bool `json:"block,omitempty"`
	}{block}
	var out MailThread
	if err := c.Do(ctx, http.MethodPost, "/mail/"+id+"/discard", nil, body, &out); err != nil {
		return nil, fmt.Errorf("discarding mail %s: %w", id, err)
	}
	return &out, nil
}
