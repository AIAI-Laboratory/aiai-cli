package screens

import (
	"strings"
)

func UpdatePrompt(currentVersion, latestVersion, changelog string, cursor int, t Theme) string {
	var s strings.Builder

	s.WriteString(t.Heading("Update available", ""))
	s.WriteString(t.Good("● Current: ") + currentVersion + "  ➔  " + t.Accent("Latest: "+latestVersion) + "\n\n")

	if changelog != "" {
		trimmed := strings.TrimSpace(changelog)
		lines := strings.Split(trimmed, "\n")
		if len(lines) > 6 {
			trimmed = strings.Join(lines[:6], "\n") + "\n…"
		}
		lines = strings.Split(trimmed, "\n")
		for i := range lines {
			lines[i] = t.Truncate(lines[i], t.Width())
		}
		trimmed = strings.Join(lines, "\n")
		s.WriteString(t.Muted("Release highlights:") + "\n" + t.Muted(trimmed) + "\n\n")
	}

	s.WriteString(t.Choice("Update now", "Download and upgrade in-place", cursor == 0))
	s.WriteString(t.Choice("Remind me later", "Skip for now, check again in 24 hours", cursor == 1))
	s.WriteString(t.Choice("Skip this version", "Do not notify again for "+latestVersion, cursor == 2))

	return strings.TrimSuffix(s.String(), "\n")
}

func Updating(latestVersion, step string, t Theme) string {
	title := "Updating AIAI CLI"
	if latestVersion != "" {
		title += " · " + latestVersion
	}

	body := t.Heading(title, "")
	if step != "" {
		body += step
	}
	return body
}

func UpdateResult(latestVersion string, success bool, err error, cursor int, t Theme) string {
	var s strings.Builder
	if success {
		title := "Updated to " + latestVersion
		if latestVersion == "" {
			title = "Up to date"
			return t.Heading(title, "") + t.Choice("Return to home", "", true)
		}
		s.WriteString(t.Heading(title, ""))
		s.WriteString(t.Choice("Restart now", "Immediately relaunch AIAI CLI", cursor == 0))
		s.WriteString(t.Choice("Exit to terminal", "Return to your shell prompt", cursor == 1))
	} else {
		s.WriteString(t.Heading("Update failed", ""))
		errMsg := "unknown error"
		if err != nil {
			errMsg = err.Error()
		}
		s.WriteString(t.Accent("! "+errMsg) + "\n\n")
		s.WriteString(t.Choice("Return to home", "", true))
	}
	return strings.TrimSuffix(s.String(), "\n")
}
