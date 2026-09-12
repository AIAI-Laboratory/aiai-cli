package screens

import (
	"strings"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
)

func DeviceLogin(device auth.DeviceAuthorization, t Theme) string {
	body := t.Heading("Sign in with GitHub", "Complete authorization in your browser.") +
		"Open: " + device.VerificationURI + "\n\n" +
		t.Title(device.UserCode) + "\n\n" +
		t.Muted("Waiting for authorization…")
	if device.BrowserOpenWarning != "" {
		body += "\n\n" + t.Warn(device.BrowserOpenWarning)
	}
	return body
}

func AuthProfile(user auth.User, storage string, t Theme) string {
	var body strings.Builder
	body.WriteString(t.Heading("Signed in", "Your AIAI account is linked to GitHub."))
	body.WriteString(t.Title("@" + user.Login))
	body.WriteString("\n")
	if user.Name != "" {
		body.WriteString(user.Name + "\n")
	}
	body.WriteString("GitHub ID: " + user.GitHubID + "\n")
	if storage != "" {
		body.WriteString("\n" + t.Muted("Credential store: "+storage))
	}
	return body.String()
}

func AuthResult(message, warning string, success bool, t Theme) string {
	if success {
		message = t.Good(message)
	} else {
		message = t.Warn(message)
	}
	body := t.Heading("Authentication", "AIAI account session") + message
	if warning != "" {
		body += "\n\n" + t.Warn(warning)
	}
	return body
}
