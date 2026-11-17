package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// BoardInfo is a board as GET /boards lists it.
type BoardInfo struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Status     string      `json:"status"`     // active, on_hold, completed
	Repository *string     `json:"repository"` // owner/name
	Language   string      `json:"language"`   // en, es
	Parent     *BoardRef   `json:"parent"`     // nil for a top-level board.
	Columns    []Column    `json:"columns"`    // In the board's order.
	Details    []NameValue `json:"details"`    // The board's own, not its parents'.
	Areas      []Area      `json:"areas"`      // Its own: who is in charge of what.
	URL        string      `json:"url"`
	// The minutes Trk's agent starts a turn with and the most it can grow
	// to, as the board itself sets them: nil inherits from the boards
	// above.
	AgentMinutes    *int `json:"agent_minutes"`
	AgentMaxMinutes *int `json:"agent_max_minutes"`
	// AgentTime is the time in effect, inherited where the board sets
	// none; nil from a server that predates it.
	AgentTime *AgentTime `json:"agent_time"`
}

// AgentTime is how long Trk's agent works a turn on a board.
type AgentTime struct {
	Minutes    int `json:"minutes"`
	MaxMinutes int `json:"max_minutes"`
}

// BoardRef names a board.
type BoardRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// NameValue is a named item, such as a board detail.
type NameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// BoardPage is one page of boards, in the team's order.
type BoardPage struct {
	Items         []BoardInfo `json:"items"`
	NextPageToken *string     `json:"next_page_token"`
}

// Boards lists the organization's boards. limit 0 is the server's
// default.
func (c *Client) Boards(ctx context.Context, pageToken string, limit int) (*BoardPage, error) {
	q := url.Values{}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out BoardPage
	if err := c.Do(ctx, http.MethodGet, "/boards", q, nil, &out); err != nil {
		return nil, fmt.Errorf("listing boards: %w", err)
	}
	return &out, nil
}

// NewBoard is a board to create. Name is required; empty options are
// left out.
type NewBoard struct {
	Name     string      `json:"name"`
	Parent   string      `json:"parent,omitempty"`   // An id or a name: a subproject of it.
	Repo     string      `json:"repo,omitempty"`     // owner/name
	Language string      `json:"language,omitempty"` // en, es
	Status   string      `json:"status,omitempty"`
	Details  []NameValue `json:"details,omitempty"`
	Areas    []Area      `json:"areas,omitempty"`
	// Unset inherits from the boards above.
	AgentMinutes    *int `json:"agent_minutes,omitempty"`
	AgentMaxMinutes *int `json:"agent_max_minutes,omitempty"`
}

// CreateBoard creates a board.
func (c *Client) CreateBoard(ctx context.Context, n NewBoard) (*BoardInfo, error) {
	var out BoardInfo
	if err := c.Do(ctx, http.MethodPost, "/boards", nil, n, &out); err != nil {
		return nil, fmt.Errorf("creating the board: %w", err)
	}
	return &out, nil
}

// BoardUpdate is a change to a board: only the fields that are not nil
// are sent. Details is the whole new list. An empty Parent takes the
// board out of its parent.
type BoardUpdate struct {
	Name     *string      `json:"name,omitempty"`
	Parent   *string      `json:"parent,omitempty"`
	Repo     *string      `json:"repo,omitempty"`
	Language *string      `json:"language,omitempty"`
	Status   *string      `json:"status,omitempty"`
	Details  *[]NameValue `json:"details,omitempty"`
	Areas    *[]Area      `json:"areas,omitempty"` // The whole new list.
	// The agent's minutes: a nil *int sends null, which clears the
	// board's own so it inherits from the boards above.
	AgentMinutes    **int `json:"agent_minutes,omitempty"`
	AgentMaxMinutes **int `json:"agent_max_minutes,omitempty"`
}

// UpdateBoard changes board (an id or a name).
func (c *Client) UpdateBoard(ctx context.Context, board string, u BoardUpdate) (*BoardInfo, error) {
	var out BoardInfo
	if err := c.Do(ctx, http.MethodPatch, "/boards/"+url.PathEscape(board), nil, u, &out); err != nil {
		return nil, fmt.Errorf("changing board %s: %w", board, err)
	}
	return &out, nil
}
