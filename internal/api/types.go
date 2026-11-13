package api

import "time"

// The API's JSON, decoded. Unknown fields are ignored, so a server that
// adds fields never breaks an older CLI. Values that may be null are
// pointers, so they encode back as null in trk's own --json.

// DeviceCode starts the device flow (RFC 8628 section 3.2).
type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"` // Seconds.
	Interval                int    `json:"interval"`   // Seconds between polls.
}

// TokenGrant is the device flow's successful answer.
type TokenGrant struct {
	AccessToken Token     `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Me is who the session belongs to.
type Me struct {
	User         User         `json:"user"`
	Organization Organization `json:"organization"`
	Session      Session      `json:"session"`
}

// User is a person in trk.
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Organization is the one organization a session acts in.
type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Session describes the token in use.
type Session struct {
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CardPage is one page of cards.
type CardPage struct {
	Items         []Card  `json:"items"`
	NextPageToken *string `json:"next_page_token"`
}

// Card is a card. In lists, Description, Customers, Attachments and
// Messages are left out.
type Card struct {
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Description *string `json:"description,omitzero"` // Absent in lists.
	Type        *string `json:"type"`                 // question, feature, bug, security
	Priority    string  `json:"priority"`             // low, normal, high, urgent
	State       string  `json:"state"`                // open, review, waiting
	DueOn       *string `json:"due_on"`               // YYYY-MM-DD
	// DueWindow is overdue, week or later, computed by the server as the
	// web's "My cards" does, in the app's time zone; nil without a date.
	DueWindow   *string      `json:"due_window"`
	ClosedAt    *time.Time   `json:"closed_at"`
	ArchivedAt  *time.Time   `json:"archived_at"`
	Agent       bool         `json:"agent"`
	Board       Board        `json:"board"`
	Column      Column       `json:"column"`
	Owners      []Person     `json:"owners"`
	Tags        []Tag        `json:"tags"`
	Customers   []Customer   `json:"customers,omitzero"` // Absent in lists.
	Branch      *string      `json:"branch"`             // nil on a board without a repository.
	PullRequest *PullRequest `json:"pull_request"`

	Attachments              []Attachment `json:"attachments,omitzero"` // Absent in lists.
	AttachmentsNextPageToken *string      `json:"attachments_next_page_token"`
	Messages                 []Message    `json:"messages,omitzero"` // The newest page, oldest first; absent in lists.
	MessagesNextPageToken    *string      `json:"messages_next_page_token"`

	InsertedAt time.Time `json:"inserted_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Board is where a card lives.
type Board struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Status     string   `json:"status"`     // active, on_hold, completed
	Repository *string  `json:"repository"` // owner/name
	Path       []string `json:"path"`       // Names, outermost first, ending in this board.
}

// Column is a board's column; Role is todo, doing, review, done or "".
type Column struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// Person is an owner of a card.
type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Tag is a card's tag.
type Tag struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Customer is a card's customer. Notes are internal: they are shown and
// written to card.md, never anywhere else.
type Customer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Notes string `json:"notes"`
}

// PullRequest is a card's pull request.
type PullRequest struct {
	URL      string  `json:"url"`
	Number   int     `json:"number"`
	State    string  `json:"state"`
	Checks   *string `json:"checks"` // pending, passing, failing
	OpenedBy string  `json:"opened_by"`
}

// Attachment is a file on a card. Path ("attachments/<name>") is where
// the server's Markdown links it; it comes from the server and is
// untrusted.
type Attachment struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	Path        string    `json:"path"`
	ContentType string    `json:"content_type"`
	ByteSize    int64     `json:"byte_size"`
	MessageID   *string   `json:"message_id"`
	URL         string    `json:"url"`
	InsertedAt  time.Time `json:"inserted_at"`
}

// Message is one message in a card's conversation.
type Message struct {
	ID         string    `json:"id"`
	Author     string    `json:"author"`
	Body       string    `json:"body"`
	InsertedAt time.Time `json:"inserted_at"`
}

// TakeResult is what taking a card changed.
type TakeResult struct {
	Card   Card      `json:"card"`
	Steps  TakeSteps `json:"steps"`
	Branch *string   `json:"branch"` // nil on a board without a repository.
}

// TakeSteps reports each step of taking a card.
type TakeSteps struct {
	Owner string `json:"owner"` // added, already
	Agent string `json:"agent"` // turned_off, already_off
	Moved string `json:"moved"` // to_doing, already_there, no_doing_column
}
