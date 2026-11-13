// Package auth signs this machine in to trk with the OAuth 2.0 Device
// Authorization Grant (RFC 8628), and keeps the resulting token.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cirope/trk-cli/internal/api"
)

// Errors that end signing in.
var (
	ErrDenied  = errors.New("the request was denied in the browser")
	ErrExpired = errors.New("the code expired before it was approved")
)

const (
	defaultInterval = 5 * time.Second // RFC 8628 3.2
	slowDownStep    = 5 * time.Second // RFC 8628 3.5
)

// Device is the part of the API the device flow needs.
type Device interface {
	DeviceCode(ctx context.Context, clientName string) (*api.DeviceCode, error)
	DeviceToken(ctx context.Context, deviceCode string) (*api.TokenGrant, error)
}

// Login asks for a device code, calls show so the person can approve it,
// then polls until it is approved, denied, expired, or ctx is done.
func Login(ctx context.Context, d Device, clientName string, show func(*api.DeviceCode)) (*api.TokenGrant, error) {
	code, err := d.DeviceCode(ctx, clientName)
	if err != nil {
		return nil, err
	}
	show(code)

	interval := time.Duration(code.Interval) * time.Second
	if interval <= 0 {
		interval = defaultInterval
	}
	deadline := time.Now().Add(time.Duration(code.ExpiresIn) * time.Second)

	for {
		if time.Now().Add(interval).After(deadline) {
			return nil, ErrExpired
		}
		if err := wait(ctx, interval); err != nil {
			return nil, err
		}
		grant, err := d.DeviceToken(ctx, code.DeviceCode)
		if err == nil {
			return grant, nil
		}
		var apiErr *api.Error
		if !errors.As(err, &apiErr) {
			return nil, fmt.Errorf("waiting for approval: %w", err)
		}
		switch apiErr.Code {
		case "authorization_pending":
		case "slow_down":
			interval += slowDownStep
		case "access_denied":
			return nil, ErrDenied
		case "expired_token":
			return nil, ErrExpired
		default:
			return nil, fmt.Errorf("waiting for approval: %w", err)
		}
	}
}

func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
