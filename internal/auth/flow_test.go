package auth

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cirope/trk-cli/internal/api"
)

// fakeDevice answers DeviceToken from a script of codes ("" = success)
// and records when each poll happened.
type fakeDevice struct {
	code   api.DeviceCode
	script []string
	polls  []time.Time
	start  time.Time
}

func (f *fakeDevice) DeviceCode(context.Context, string) (*api.DeviceCode, error) {
	f.start = time.Now()
	c := f.code
	return &c, nil
}

func (f *fakeDevice) DeviceToken(_ context.Context, deviceCode string) (*api.TokenGrant, error) {
	if deviceCode != f.code.DeviceCode {
		return nil, errors.New("wrong device code")
	}
	f.polls = append(f.polls, time.Now())
	next := f.script[0]
	if len(f.script) > 1 {
		f.script = f.script[1:]
	}
	if next == "" {
		return &api.TokenGrant{AccessToken: "tok", TokenType: "Bearer"}, nil
	}
	return nil, &api.Error{Status: 400, Code: next, Message: next}
}

// offsets returns each poll's time since the flow started, in seconds.
func (f *fakeDevice) offsets() []int {
	var out []int
	for _, p := range f.polls {
		out = append(out, int(p.Sub(f.start)/time.Second))
	}
	return out
}

func newFake(script ...string) *fakeDevice {
	return &fakeDevice{
		code: api.DeviceCode{
			DeviceCode: "dc", UserCode: "WDJB-MJHT",
			VerificationURIComplete: "https://trk.test/cli/approve?code=WDJB-MJHT",
			Interval:                5, ExpiresIn: 60,
		},
		script: script,
	}
}

func TestLoginPollsUntilApproved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake("authorization_pending", "authorization_pending", "")
		var shown *api.DeviceCode
		grant, err := Login(t.Context(), f, "test-host", func(c *api.DeviceCode) { shown = c })
		if err != nil {
			t.Fatal(err)
		}
		if grant.AccessToken.Reveal() != "tok" {
			t.Errorf("token = %q", grant.AccessToken.Reveal())
		}
		if shown == nil || shown.UserCode != "WDJB-MJHT" {
			t.Errorf("shown = %+v, want the code shown before polling", shown)
		}
		if got := f.offsets(); !slices.Equal(got, []int{5, 10, 15}) {
			t.Errorf("polled at %v s, want every 5 s", got)
		}
	})
}

func TestLoginSlowsDown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake("slow_down", "authorization_pending", "")
		if _, err := Login(t.Context(), f, "h", func(*api.DeviceCode) {}); err != nil {
			t.Fatal(err)
		}
		// RFC 8628 3.5: slow_down adds 5 seconds to the interval for good.
		if got := f.offsets(); !slices.Equal(got, []int{5, 15, 25}) {
			t.Errorf("polled at %v s, want 5, 15, 25", got)
		}
	})
}

func TestLoginDefaultsAMissingInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake("")
		f.code.Interval = 0
		if _, err := Login(t.Context(), f, "h", func(*api.DeviceCode) {}); err != nil {
			t.Fatal(err)
		}
		if got := f.offsets(); !slices.Equal(got, []int{5}) {
			t.Errorf("polled at %v s, want 5 (the RFC's default)", got)
		}
	})
}

func TestLoginEnds(t *testing.T) {
	tests := []struct {
		name   string
		script []string
		want   error
	}{
		{"denied", []string{"authorization_pending", "access_denied"}, ErrDenied},
		{"expired by the server", []string{"expired_token"}, ErrExpired},
		{"expired by the clock", []string{"authorization_pending"}, ErrExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newFake(tt.script...)
				_, err := Login(t.Context(), f, "h", func(*api.DeviceCode) {})
				if !errors.Is(err, tt.want) {
					t.Errorf("err = %v, want %v", err, tt.want)
				}
				if len(f.polls) > 12 {
					t.Errorf("polled %d times after a 60 s code expired", len(f.polls))
				}
			})
		})
	}
}

func TestLoginStopsOnCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake("authorization_pending")
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(7*time.Second, cancel)
		_, err := Login(ctx, f, "h", func(*api.DeviceCode) {})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		if len(f.polls) != 1 {
			t.Errorf("polled %d times, want 1 before the cancel at 7 s", len(f.polls))
		}
	})
}

func TestLoginOtherErrorsEndTheFlow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake("server_error")
		_, err := Login(t.Context(), f, "h", func(*api.DeviceCode) {})
		var apiErr *api.Error
		if !errors.As(err, &apiErr) || apiErr.Code != "server_error" {
			t.Errorf("err = %v, want the server's error", err)
		}
	})
}
