package screens

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
)

func ServerList(servers []sshmanager.Server, cursor int, connect bool, filter string, t Theme) string {
	title := "SSH servers"
	if connect {
		title = "Connect with SSH"
	}
	var body strings.Builder
	body.WriteString(t.Heading(title, ""))
	if len(servers) == 0 {
		if filter != "" {
			body.WriteString(t.Muted("No servers match " + strconv.Quote(filter) + ".\n\n"))
			body.WriteString("Press " + t.Accent("Esc") + " to clear the filter.")
			return body.String()
		}
		body.WriteString(t.Muted("No SSH servers configured.\n\n"))
		body.WriteString("Add one from your shell:\n\n")
		body.WriteString(t.Accent("aiai server add production --host example.com --user deploy"))
		return body.String()
	}
	for i, server := range servers {
		body.WriteString(t.Choice(server.Alias, serverDestination(server), i == cursor))
	}
	return strings.TrimSuffix(body.String(), "\n")
}

func ServerDetail(server sshmanager.Server, t Theme) string {
	return t.Heading(server.Alias, "") + serverDetailBody(server, t, t.Width())
}

func serverDestination(server sshmanager.Server) string {
	destination := server.Username + "@" + server.Host
	if server.Port != 22 {
		destination += ":" + strconv.Itoa(server.Port)
	}
	return destination
}

func serverDetailBody(server sshmanager.Server, t Theme, width int) string {
	key := "OpenSSH config / ssh-agent"
	if server.IdentityFile != "" {
		key = server.IdentityFile
	}
	valueWidth := max(1, width-12)
	body := t.Section("Connection") + "\n" +
		"  Host      " + t.Truncate(server.Host, valueWidth) + "\n" +
		"  User      " + t.Truncate(server.Username, valueWidth) + "\n" +
		"  Port      " + strconv.Itoa(server.Port) + "\n\n" +
		t.Section("Identity") + "\n" +
		"  " + t.Truncate(key, max(1, width-2)) + "\n"
	if server.Description != "" {
		body += "\n" + t.Section("Notes") + "\n  " + t.Truncate(server.Description, max(1, width-2)) + "\n"
	}
	if !server.UpdatedAt.IsZero() {
		body += "\n" + t.Muted("Updated "+server.UpdatedAt.Local().Format("2006-01-02 15:04"))
	}
	return body
}

func AccessLogs(logs []sshmanager.AccessLog, t Theme) string {
	var body strings.Builder
	body.WriteString(t.Heading("SSH access history", "Newest first · local time"))
	if len(logs) == 0 {
		body.WriteString(t.Muted("No SSH sessions recorded yet.\n\n"))
		body.WriteString("Connect with " + t.Accent("/ssh") + " and completed sessions will appear here.")
		return body.String()
	}
	if t.Width() >= 72 {
		serverWidth := min(18, max(12, t.Width()/5))
		destinationWidth := max(16, t.Width()-serverWidth-41)
		fmt.Fprintf(&body, "%s  %s  %s  %s  %s\n",
			logCell(t.Section("STATUS"), 9), logCell(t.Section("SERVER"), serverWidth),
			logCell(t.Section("DESTINATION"), destinationWidth), logCell(t.Section("STARTED"), 16), logCell(t.Section("DURATION"), 8))
		body.WriteString(t.Muted(strings.Repeat("─", t.Width())) + "\n")
		for _, entry := range logs {
			fmt.Fprintf(&body, "%s  %s  %s  %s  %s\n",
				logCell(logStatus(entry, t), 9), logCell(t.Truncate(entry.ServerAlias, serverWidth), serverWidth),
				logCell(t.Truncate(entry.Username+"@"+entry.Host, destinationWidth), destinationWidth),
				logCell(formatLogTime(entry.StartedAt), 16), logCell(logDuration(entry), 8))
		}
		return strings.TrimSuffix(body.String(), "\n")
	}
	for _, entry := range logs {
		status := logStatus(entry, t)
		duration := "-"
		if entry.DurationMS != nil {
			duration = (time.Duration(*entry.DurationMS) * time.Millisecond).Round(time.Millisecond).String()
		}
		exit := "-"
		if entry.ExitCode != nil {
			exit = strconv.Itoa(*entry.ExitCode)
		}
		fmt.Fprintf(&body, "%s  %s\n", t.Title(entry.ServerAlias), status)
		fmt.Fprintf(&body, "  %s@%s\n", entry.Username, entry.Host)
		fmt.Fprintf(&body, "  %s · %s · exit %s\n\n", t.Muted(formatLogTime(entry.StartedAt)), duration, exit)
	}
	return strings.TrimSuffix(body.String(), "\n")
}

func logCell(value string, width int) string {
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(value)
}

func logStatus(entry sshmanager.AccessLog, t Theme) string {
	label := "• " + entry.Status
	switch entry.Status {
	case sshmanager.StatusSuccess:
		return t.Good("✓ success")
	case sshmanager.StatusFailed:
		if entry.ExitCode != nil {
			label = "× failed"
		}
		return t.Accent(label)
	case sshmanager.StatusCanceled:
		return t.Warn("• canceled")
	default:
		return t.Warn(label)
	}
}

func logDuration(entry sshmanager.AccessLog) string {
	if entry.DurationMS == nil {
		return "-"
	}
	return (time.Duration(*entry.DurationMS) * time.Millisecond).Round(time.Millisecond).String()
}

func formatLogTime(started time.Time) string {
	local := started.Local()
	now := time.Now()
	if local.Year() == now.Year() && local.YearDay() == now.YearDay() {
		return "Today " + local.Format("15:04")
	}
	if local.Year() == now.AddDate(0, 0, -1).Year() && local.YearDay() == now.AddDate(0, 0, -1).YearDay() {
		return "Yesterday " + local.Format("15:04")
	}
	return local.Format("2006-01-02 15:04")
}

func SSHFailure(err error, t Theme) string {
	message := "SSH management is unavailable."
	if err != nil {
		message = err.Error()
	}
	return t.Heading("SSH", "") + t.Warn(message)
}

func KeyList(keys []sshmanager.Key, query, filter, notice string, cursor int, t Theme) string {
	subtitle := ""
	if query != "" {
		subtitle = "Results for " + strconv.Quote(query)
	}
	var body strings.Builder
	body.WriteString(t.Heading("SSH keys", subtitle))
	if notice != "" {
		body.WriteString(t.Good(notice) + "\n\n")
	}
	if len(keys) == 0 {
		if filter != "" {
			body.WriteString(t.Muted("No keys match " + strconv.Quote(filter) + ".\n\n"))
			body.WriteString("Press " + t.Accent("Esc") + " to clear the filter.")
			return body.String()
		}
		body.WriteString(t.Muted("No matching SSH key pairs found.\n\n"))
		body.WriteString("Generate one from the TUI command bar:\n\n")
		body.WriteString(t.Accent("/keygen my-key"))
		return body.String()
	}
	for i, key := range keys {
		source := "external"
		if key.Managed {
			source = "managed"
		}
		description := strings.TrimPrefix(key.Type, "ssh-") + " · " + source
		body.WriteString(t.Choice(key.Name, description, i == cursor))
	}
	return strings.TrimSuffix(body.String(), "\n")
}

func KeyDetail(key sshmanager.Key, notice string, t Theme) string {
	source := "External ~/.ssh key"
	if key.Managed {
		source = "AIAI managed key"
	}
	body := t.Heading(key.Name, source)
	if notice != "" {
		body += t.Good(notice) + "\n\n"
	}
	valueWidth := max(1, t.Width()-15)
	body += t.Section("Identity") + "\n" +
		"  Type         " + t.Truncate(key.Type, valueWidth) + "\n" +
		"  Fingerprint  " + t.Truncate(key.Fingerprint, valueWidth) + "\n\n" +
		t.Section("Files") + "\n" +
		"  Private      " + t.Truncate(key.PrivatePath, valueWidth) + "\n" +
		"  Public       " + t.Truncate(key.PublicPath, valueWidth) + "\n"
	if key.Comment != "" {
		body += "  Comment      " + t.Truncate(key.Comment, valueWidth) + "\n"
	}
	body += "\n" + t.Section("Actions") + "\n" + t.Accent("c") + "  Copy public key\n"
	if key.Managed {
		body += t.Accent("d") + "  Delete managed key\n"
	}
	body += "\n" + t.Muted("Private key contents are never displayed or copied into AIAI storage.")
	return body
}

func KeyDeleteConfirmation(key sshmanager.Key, yes bool, t Theme) string {
	return t.Heading("Delete "+key.Name+"?", "Both the private and public key files will be permanently removed.") +
		t.Warn(key.PrivatePath+"\n"+key.PublicPath) + "\n\n" +
		t.Choice("No, keep this key", "", !yes) +
		t.Choice("Yes, delete both files", "", yes)
}
