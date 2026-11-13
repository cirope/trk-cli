package cli

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/cirope/trk-cli/internal/api"
)

// parseCardRef returns the card id in s: an id, or a card's URL as copied
// from the browser (…/boards/<board id>/cards/<card id>, any query).
func parseCardRef(s string) (string, error) {
	s = strings.TrimSpace(s)
	if api.ValidID(s) {
		return s, nil
	}
	bad := usageError{fmt.Errorf("%q is not a card id or a card's URL", s)}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", bad
	}
	parts := strings.Split(strings.TrimSuffix(u.Path, "/"), "/")
	n := len(parts)
	if n < 4 || parts[n-4] != "boards" || parts[n-2] != "cards" ||
		!api.ValidID(parts[n-3]) || !api.ValidID(parts[n-1]) {
		return "", bad
	}
	return parts[n-1], nil
}
