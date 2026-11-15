package cli

import "testing"

func TestLinkTarget(t *testing.T) {
	tests := []struct {
		in, kind, value string
	}{
		{"https://github.com/cirope/trk/pull/12", "pull", "https://github.com/cirope/trk/pull/12"},
		{"https://github.com/cirope/trk/pull/12/files", "pull", "https://github.com/cirope/trk/pull/12"},
		{"https://github.com/cirope/trk/commit/4f2a9c1e8b7d", "commit", "https://github.com/cirope/trk/commit/4f2a9c1e8b7d"},
		{"4f2a9c1", "commit", "4f2a9c1"},
		{"4F2A9C1E", "commit", "4F2A9C1E"},
		{"HEAD", "ref", "HEAD"},
		{"main~2", "ref", "main~2"},
		{"https://github.com/cirope/trk/issues/3", "", ""},
		{"-x", "", ""},
	}
	for _, tt := range tests {
		kind, value := linkTarget(tt.in)
		if kind != tt.kind || value != tt.value {
			t.Errorf("linkTarget(%q) = %q, %q; want %q, %q", tt.in, kind, value, tt.kind, tt.value)
		}
	}
}
