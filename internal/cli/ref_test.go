package cli

import (
	"strings"
	"testing"

	"github.com/cirope/trk-cli/internal/api"
)

func TestParseCardRef(t *testing.T) {
	good := map[string]string{
		"BaHtsbhzVDa9":   "BaHtsbhzVDa9",
		" BaHtsbhzVDa9 ": "BaHtsbhzVDa9",
		"https://trk.hullop.com/boards/TyzgACHc57CG/cards/BaHtsbhzVDa9":             "BaHtsbhzVDa9",
		"https://trk.hullop.com/boards/TyzgACHc57CG/cards/BaHtsbhzVDa9/":            "BaHtsbhzVDa9",
		"https://trk.hullop.com/boards/TyzgACHc57CG/cards/BaHtsbhzVDa9?tab=files#x": "BaHtsbhzVDa9",
		"http://localhost:4000/boards/TyzgACHc57CG/cards/BaHtsbhzVDa9":              "BaHtsbhzVDa9",
		"https://example.com/trk/boards/TyzgACHc57CG/cards/BaHtsbhzVDa9?state=open": "BaHtsbhzVDa9",
	}
	for in, want := range good {
		got, err := parseCardRef(in)
		if err != nil || got != want {
			t.Errorf("parseCardRef(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{
		"", "BaHtsbhzVDa", "BaHtsbhzVDa0", "../me", "BaHtsbhzVDa9/../me",
		"https://trk.hullop.com/cards/BaHtsbhzVDa9",
		"https://trk.hullop.com/boards/TyzgACHc57CG/cards/../../me",
		"https://trk.hullop.com/boards/TyzgACHc57CG/cards/BaHtsbhzVDa9/messages",
		"https://trk.hullop.com/boards/TyzgACHc57CG",
		"ftp://trk.hullop.com/boards/TyzgACHc57CG/cards/BaHtsbhzVDa9",
		"file:///boards/TyzgACHc57CG/cards/BaHtsbhzVDa9",
	}
	for _, in := range bad {
		if got, err := parseCardRef(in); err == nil {
			t.Errorf("parseCardRef(%q) = %q, want an error", in, got)
		} else if exitCode(err) != ExitUsage {
			t.Errorf("parseCardRef(%q) error exits %d, want %d", in, exitCode(err), ExitUsage)
		}
	}
}

func FuzzParseCardRef(f *testing.F) {
	f.Add("BaHtsbhzVDa9")
	f.Add("https://trk.hullop.com/boards/TyzgACHc57CG/cards/BaHtsbhzVDa9?tab=x")
	f.Add("../../etc/passwd")
	f.Fuzz(func(t *testing.T, in string) {
		id, err := parseCardRef(in)
		if err != nil {
			return
		}
		if !api.ValidID(id) {
			t.Fatalf("parseCardRef(%q) = %q, not a valid id", in, id)
		}
		if !strings.Contains(in, id) {
			t.Fatalf("parseCardRef(%q) = %q, not from the input", in, id)
		}
	})
}

func TestOpenBrowserRefusesNonHTTP(t *testing.T) {
	for _, u := range []string{"-a Calculator", "file:///etc/passwd", "javascript:alert(1)", "https://"} {
		if err := OpenBrowser(u); err == nil {
			t.Errorf("OpenBrowser(%q) succeeded", u)
		}
	}
}
