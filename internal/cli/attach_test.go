package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckUploadsLimits(t *testing.T) {
	dir := t.TempDir()
	sized := func(name string, size int64) string {
		p := filepath.Join(dir, name)
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(size); err != nil { // Sparse: no 20 MB written.
			t.Fatal(err)
		}
		_ = f.Close()
		return p
	}
	if _, err := checkUploads([]string{sized("ok.bin", maxUpload)}); err != nil {
		t.Errorf("a file of exactly 20 MB: %v", err)
	}
	_, err := checkUploads([]string{sized("big.bin", maxUpload+1)})
	if exitCode(err) != ExitUsage || !strings.Contains(err.Error(), "big.bin is over the 20 MB") {
		t.Errorf("a file over 20 MB: %v", err)
	}
	var many []string
	for i := range 6 {
		many = append(many, sized(string(rune('a'+i))+".bin", maxUpload))
	}
	if _, err := checkUploads(many); exitCode(err) != ExitUsage || !strings.Contains(err.Error(), "100 MB") {
		t.Errorf("120 MB in all: %v", err)
	}
}
