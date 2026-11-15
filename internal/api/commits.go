package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Commit is a commit on a card: one that named the card on the default
// branch, or one linked by hand.
type Commit struct {
	Sha         string       `json:"sha"`
	Subject     string       `json:"subject"`
	Author      CommitAuthor `json:"author"`
	URL         string       `json:"url"`
	CommittedAt time.Time    `json:"committed_at"`
	Closes      bool         `json:"closes"` // Its message closes the card ("Closes #<id>").
	Checks      *string      `json:"checks"` // pending, passing, failing
	Via         string       `json:"via"`    // How it reached the card: link, or by naming it.
}

// CommitAuthor is who wrote a commit.
type CommitAuthor struct {
	Name  string `json:"name"`
	Login string `json:"login"`
}

// LinkCommit links a commit of the board's repository, on its default
// branch, to card id: commit is a sha (7+ hex) or a GitHub commit URL.
// Linking never closes the card.
func (c *Client) LinkCommit(ctx context.Context, id, commit string) (*Commit, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out Commit
	body := map[string]string{"commit": commit}
	if err := c.Do(ctx, http.MethodPost, "/cards/"+id+"/commits", nil, body, &out); err != nil {
		return nil, fmt.Errorf("linking commit %s to card %s: %w", commit, id, err)
	}
	return &out, nil
}

// CommitPage is one page of a card's commits, newest first.
type CommitPage struct {
	Items         []Commit `json:"items"`
	NextPageToken *string  `json:"next_page_token"`
}

// CardCommits returns the page of card id's commits at pageToken (""
// for the first).
func (c *Client) CardCommits(ctx context.Context, id, pageToken string) (*CommitPage, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	q := url.Values{}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	var out CommitPage
	if err := c.Do(ctx, http.MethodGet, "/cards/"+id+"/commits", q, nil, &out); err != nil {
		return nil, fmt.Errorf("listing the commits of card %s: %w", id, err)
	}
	return &out, nil
}
