package cli

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

// OpenBrowser opens an http(s) URL in the default browser. Failing is not
// an error for callers: the URL is always printed too. URLs come from the
// server, so anything else (an option-like "-a App", a file: URL) is
// refused rather than handed to the opener.
func OpenBrowser(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("not opening %q: not an http(s) URL", rawURL)
	}
	url := u.String()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) //nolint:gosec,noctx // An http(s) URL, checked above; the browser outlives trk.
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec,noctx // Checked above.
	default:
		cmd = exec.Command("xdg-open", url) //nolint:gosec,noctx // Checked above.
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
