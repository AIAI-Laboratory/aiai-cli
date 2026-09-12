package auth

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

func OpenBrowser(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("refuse to open invalid verification URL")
	}
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{rawURL}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		command, args = "xdg-open", []string{rawURL}
	}
	if err := exec.Command(command, args...).Start(); err != nil { //nolint:gosec // Fixed command selected by GOOS.
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
