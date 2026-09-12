package screens

import (
	"strings"
)

func UpdatePrompt(currentVersion, latestVersion, changelog string, cursor int, t Theme) string {
	var s strings.Builder

	s.WriteString(t.Heading("New version available!", "A fresh release of AIAI CLI is ready to install."))
	s.WriteString(t.Good("● Current: ") + currentVersion + "  ➔  " + t.Accent("Latest: "+latestVersion) + "\n\n")

	if changelog != "" {
		trimmed := strings.TrimSpace(changelog)
		lines := strings.Split(trimmed, "\n")
		if len(lines) > 6 {
			trimmed = strings.Join(lines[:6], "\n") + "\n…"
		}
		s.WriteString(t.Muted("Release highlights:") + "\n" + t.Muted(trimmed) + "\n\n")
	}

	s.WriteString(t.Choice("Update now", "Download and upgrade in-place", cursor == 0))
	s.WriteString(t.Choice("Remind me later", "Skip for now, check again in 24 hours", cursor == 1))
	s.WriteString(t.Choice("Skip this version", "Do not notify again for "+latestVersion, cursor == 2))

	return strings.TrimSuffix(s.String(), "\n")
}

func Updating(latestVersion, step string, t Theme) string {
	title := "Updating AIAI CLI…"
	sub := "Upgrading to " + latestVersion
	if latestVersion == "" {
		sub = "Checking for available updates"
	}

	body := t.Heading(title, sub)
	if step != "" {
		body += t.Accent("⏳ ") + step + "\n\n"
	}
	body += t.Muted("Please keep your terminal open while the update proceeds.")
	return body
}

func UpdateResult(latestVersion string, success bool, err error, cursor int, t Theme) string {
	var s strings.Builder
	if success {
		title := "Update complete!"
		sub := "AIAI CLI has been updated to " + latestVersion
		if latestVersion == "" {
			title = "Up to date"
			sub = "AIAI CLI is already running the latest version."
		}
		s.WriteString(t.Heading(title, sub))
		s.WriteString(t.Choice("Restart now", "Immediately relaunch AIAI CLI", cursor == 0))
		s.WriteString(t.Choice("Exit to terminal", "Return to your shell prompt", cursor == 1))
	} else {
		s.WriteString(t.Heading("Update failed", "Could not complete the operation."))
		errMsg := "unknown error"
		if err != nil {
			errMsg = err.Error()
		}
		s.WriteString(t.Accent("! "+errMsg) + "\n\n")
		s.WriteString(t.Choice("Return to home", "Go back to the main menu", true))
	}
	return strings.TrimSuffix(s.String(), "\n")
}
