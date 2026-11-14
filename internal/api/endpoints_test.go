package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cirope/trk-cli/internal/api/apitest"
	"github.com/google/go-cmp/cmp"
)

func fixtureClient(t *testing.T, token Token, fixtures ...string) (*Client, *apitest.Server) {
	t.Helper()
	srv := apitest.NewServer(t, fixtures...)
	c, err := New(Options{Host: srv.URL, Token: token, HTTPClient: srv.Client(), MaxRetries: -1})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDeviceCode(t *testing.T) {
	c, srv := fixtureClient(t, "", "device.code.ok")
	got, err := c.DeviceCode(context.Background(), "test-host")
	if err != nil {
		t.Fatal(err)
	}
	want := &DeviceCode{
		DeviceCode:              "dc_test_device_code",
		UserCode:                "WDJB-MJHT",
		VerificationURI:         "http://trk.test/cli/approve",
		VerificationURIComplete: "http://trk.test/cli/approve?code=WDJB-MJHT",
		ExpiresIn:               900,
		Interval:                5,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
	if h := srv.Requests()[0].Header.Get("Authorization"); h != "" {
		t.Errorf("device code request sent Authorization %q", h)
	}
}

func TestDeviceTokenOK(t *testing.T) {
	c, _ := fixtureClient(t, "", "device.token.ok")
	got, err := c.DeviceToken(context.Background(), "dc_test_device_code")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken.Reveal() != "test_access_token" || got.TokenType != "Bearer" ||
		!got.ExpiresAt.Equal(mustTime("2027-02-11T12:00:00Z")) {
		t.Errorf("grant = %+v (token %q)", got, got.AccessToken.Reveal())
	}
}

func TestDeviceTokenErrors(t *testing.T) {
	tests := []struct {
		fixture string
		code    string
		message string
	}{
		{"device.token.pending", "authorization_pending", "authorization_pending"},
		{"device.token.slow_down", "slow_down", "slow_down"},
		{"device.token.denied", "access_denied", "access_denied"},
		{"device.token.expired", "expired_token", "expired_token"},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			c, _ := fixtureClient(t, "", tt.fixture)
			_, err := c.DeviceToken(context.Background(), "dc_test_device_code")
			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v, want *Error", err)
			}
			if apiErr.Status != 400 || apiErr.Code != tt.code || apiErr.Message != tt.message {
				t.Errorf("error = %+v, want 400 %s %q", apiErr, tt.code, tt.message)
			}
		})
	}
}

func TestMe(t *testing.T) {
	c, srv := fixtureClient(t, "test_access_token", "me.ok")
	got, err := c.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := &Me{
		User:         User{ID: "Us9rFrancoX2", Name: "Franco Catena", Email: "franco@example.com"},
		Organization: Organization{ID: "Or9HuLLop2Zx", Name: "Hullop"},
		Session:      Session{Name: "trk CLI on test-host", ExpiresAt: mustTime("2027-02-11T12:00:00Z")},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
	if h := srv.Requests()[0].Header.Get("Authorization"); h != "Bearer test_access_token" {
		t.Errorf("Authorization = %q", h)
	}
}

func TestMeUnauthorized(t *testing.T) {
	c, _ := fixtureClient(t, "test_access_token", "me.unauthorized")
	_, err := c.Me(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}

func TestRevokeSession(t *testing.T) {
	c, srv := fixtureClient(t, "test_access_token", "session.delete.ok")
	if err := c.RevokeSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r := srv.Requests()[0]; r.Method != http.MethodDelete || r.Path != "/api/v1/session" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
}

func TestCards(t *testing.T) {
	c, _ := fixtureClient(t, "test_access_token", "cards.list.ok")
	page, err := c.Cards(context.Background(), CardsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.NextPageToken != nil {
		t.Fatalf("page = %d items, next %v", len(page.Items), page.NextPageToken)
	}
	second := page.Items[1]
	if second.ID != "Xk29LmQp77aB" || second.DueOn != nil || second.Branch != nil ||
		second.Board.Repository != nil || second.Board.Status != "on_hold" {
		t.Errorf("second card = %+v", second)
	}
	if diff := cmp.Diff([]string{"Producto", "Implementación Acme"}, second.Board.Path); diff != "" {
		t.Errorf("board path (-want +got):\n%s", diff)
	}
}

func TestCardsFiltersAndPaging(t *testing.T) {
	c, _ := fixtureClient(t, "test_access_token", "cards.list.filtered", "cards.list.next_page")
	page, err := c.Cards(context.Background(), CardsQuery{
		Board: "TyzgACHc57CG", State: "open", Type: "bug", Priority: "high", All: true, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextPageToken == nil || *page.NextPageToken != "pt_next_page" {
		t.Fatalf("next page token = %v", page.NextPageToken)
	}
	page, err = c.Cards(context.Background(), CardsQuery{Limit: 1, PageToken: *page.NextPageToken})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "Xk29LmQp77aB" {
		t.Errorf("second page = %+v", page.Items)
	}
}

func TestCard(t *testing.T) {
	c, _ := fixtureClient(t, "test_access_token", "card.get.ok")
	card, err := c.Card(context.Background(), "BaHtsbhzVDa9")
	if err != nil {
		t.Fatal(err)
	}
	if card.Title != "CSV export loses accents" || card.Column.Role != "todo" || !card.Agent ||
		card.Branch == nil || *card.Branch != "trk/card-BaHtsbhzVDa9" ||
		card.DueOn == nil || *card.DueOn != "2026-11-27" || card.PullRequest != nil || card.ClosedAt != nil {
		t.Errorf("card = %+v", card)
	}
	if len(card.Customers) != 1 || card.Customers[0].Notes != "Has the new_export flag on." {
		t.Errorf("customers = %+v", card.Customers)
	}
	wantAttachment := Attachment{
		ID: "At5tRaCe9Lg2", Filename: "trace.log", Path: "attachments/trace.log", ContentType: "text/plain", ByteSize: 35,
		URL:        "http://trk.test/api/v1/attachments/At5tRaCe9Lg2",
		InsertedAt: mustTime("2026-11-12T09:05:00Z"),
	}
	if card.Attachments[0].MessageID != nil {
		t.Errorf("message id = %v, want nil", *card.Attachments[0].MessageID)
	}
	if diff := cmp.Diff([]Attachment{wantAttachment}, card.Attachments); diff != "" {
		t.Errorf("attachments (-want +got):\n%s", diff)
	}
	if len(card.Messages) != 1 || !card.Messages[0].InsertedAt.Equal(mustTime("2026-11-13T12:15:32Z")) {
		t.Errorf("messages = %+v", card.Messages)
	}
}

func TestCardNotFound(t *testing.T) {
	c, _ := fixtureClient(t, "test_access_token", "card.get.not_found")
	_, err := c.Card(context.Background(), "BaHtsbhzVDa9")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestCardMarkdown(t *testing.T) {
	c, srv := fixtureClient(t, "test_access_token", "card.markdown.ok")
	md, err := c.CardMarkdown(context.Background(), "BaHtsbhzVDa9")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(md, "# CSV export loses accents\n") || !strings.Contains(md, "(attachments/trace.log)") {
		t.Errorf("markdown = %q", md)
	}
	if a := srv.Requests()[0].Header.Get("Accept"); a != "text/markdown" {
		t.Errorf("Accept = %q, want text/markdown", a)
	}
}

func TestTake(t *testing.T) {
	c, _ := fixtureClient(t, "test_access_token", "card.take.ok")
	got, err := c.Take(context.Background(), "BaHtsbhzVDa9")
	if err != nil {
		t.Fatal(err)
	}
	want := TakeSteps{Owner: "added", Agent: "turned_off", Moved: "to_doing"}
	if got.Steps != want || got.Branch == nil || *got.Branch != "trk/card-BaHtsbhzVDa9" || got.Card.Agent || got.Card.Column.Role != "doing" {
		t.Errorf("take = %+v", got)
	}
}

func TestTakeAgentWorking(t *testing.T) {
	c, _ := fixtureClient(t, "test_access_token", "card.take.agent_working")
	_, err := c.Take(context.Background(), "BaHtsbhzVDa9")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "agent_working" || !errors.Is(err, ErrConflict) {
		t.Errorf("err = %v, want a 409 agent_working", err)
	}
}

// An id that is really a path never reaches the server.
func TestHostileIDs(t *testing.T) {
	c, srv := fixtureClient(t, "test_access_token")
	for _, id := range []string{"", "../me", "BaHtsbhzVDa9/../../me", "BaHtsbhzVDa%", "BaHtsbhzVDaI", "BaHtsbhzVDa9X", "a?b=c"} {
		if _, err := c.Card(context.Background(), id); !errors.Is(err, ErrBadID) {
			t.Errorf("Card(%q) err = %v, want ErrBadID", id, err)
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("server got %d requests, want none", n)
	}
}

func TestValidID(t *testing.T) {
	for id, want := range map[string]bool{
		"BaHtsbhzVDa9": true, "23456789ABCD": true,
		"BaHtsbhzVDa": false, "BaHtsbhzVDa0": false, "BaHtsbhzVDaO": false, "BaHtsbhzVDal": false,
	} {
		if got := ValidID(id); got != want {
			t.Errorf("ValidID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestCardAttachmentsPage(t *testing.T) {
	c, _ := fixtureClient(t, "test_access_token", "card.attachments.next_page")
	page, err := c.CardAttachments(context.Background(), "BaHtsbhzVDa9", "pt_more_files")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Path != "attachments/31-trace.log" || page.NextPageToken != nil {
		t.Errorf("page = %+v", page)
	}
}

func TestCardMessagesPage(t *testing.T) {
	c, _ := fixtureClient(t, "test_access_token", "card.messages.next_page")
	page, err := c.CardMessages(context.Background(), "BaHtsbhzVDa9", "pt_older")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Author != "human" {
		t.Errorf("page = %+v", page)
	}
}

func TestDownload(t *testing.T) {
	c, srv := fixtureClient(t, "test_access_token", "attachment.get.ok")
	var b strings.Builder
	n, err := c.Download(context.Background(), "At5tRaCe9Lg2", 35, &b)
	if err != nil {
		t.Fatal(err)
	}
	if n != 35 || b.Len() != 35 {
		t.Errorf("wrote %d (%d), want 35", n, b.Len())
	}
	r := srv.Requests()[0]
	if r.Header.Get("Authorization") != "Bearer test_access_token" {
		t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
	}
}

func TestDownloadSizeMismatch(t *testing.T) {
	for _, size := range []int64{34, 36} {
		c, _ := fixtureClient(t, "test_access_token", "attachment.get.ok")
		var b strings.Builder
		_, err := c.Download(context.Background(), "At5tRaCe9Lg2", size, &b)
		if !errors.Is(err, ErrSizeMismatch) {
			t.Errorf("size %d: err = %v, want ErrSizeMismatch", size, err)
		}
		if int64(b.Len()) > size {
			t.Errorf("size %d: wrote %d bytes, more than promised", size, b.Len())
		}
	}
}

func TestDownloadErrors(t *testing.T) {
	c, srv := fixtureClient(t, "test_access_token")
	if _, err := c.Download(context.Background(), "../../me", 1, io.Discard); !errors.Is(err, ErrBadID) {
		t.Errorf("err = %v, want ErrBadID", err)
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("server got %d requests", n)
	}
}

// A card without a type keeps its null in trk's own --json.
func TestNullTypeStaysNull(t *testing.T) {
	var c Card
	if err := json.Unmarshal([]byte(`{"type": null}`), &c); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(c)
	if !strings.Contains(string(out), `"type":null`) {
		t.Errorf("re-encoded type = %s", out)
	}
}

func TestMeUpgradeRequired(t *testing.T) {
	c, _ := fixtureClient(t, "test_access_token", "me.upgrade_required")
	_, err := c.Me(context.Background())
	if !errors.Is(err, ErrUpgradeRequired) {
		t.Errorf("err = %v, want ErrUpgradeRequired", err)
	}
}
