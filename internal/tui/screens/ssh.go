package screens

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
)

func ServerList(servers []sshmanager.Server, cursor int, connect bool, t Theme) string {
	title, subtitle := "SSH servers", "Local profiles available on this computer."
	if connect {
		title, subtitle = "Connect with SSH", "Choose a server; AIAI will release the TUI before starting OpenSSH."
	}
	var body strings.Builder
	body.WriteString(t.Heading(title, subtitle))
	if len(servers) == 0 {
		body.WriteString(t.Muted("No SSH servers configured.\n\n"))
		body.WriteString("Add one from your shell:\n\n")
		body.WriteString(t.Accent("aiai server add production --host example.com --user deploy"))
		return body.String()
	}
	for i, server := range servers {
		destination := server.Username + "@" + server.Host
		if server.Port != 22 {
			destination += ":" + strconv.Itoa(server.Port)
		}
		body.WriteString(t.Choice(server.Alias, destination, i == cursor))
	}
	return strings.TrimSuffix(body.String(), "\n")
}

func ServerDetail(server sshmanager.Server, t Theme) string {
	key := "OpenSSH config / ssh-agent"
	if server.IdentityFile != "" {
		key = server.IdentityFile
	}
	body := t.Heading(server.Alias, "SSH server profile") +
		"Host: " + server.Host + "\n" +
		"Port: " + strconv.Itoa(server.Port) + "\n" +
		"User: " + server.Username + "\n" +
		"Key: " + key + "\n"
	if server.Description != "" {
		body += "Description: " + server.Description + "\n"
	}
	body += "\n" + t.Muted("Created: "+server.CreatedAt.Local().Format(time.RFC3339)) + "\n" +
		t.Muted("Updated: "+server.UpdatedAt.Local().Format(time.RFC3339))
	return body
}

func AccessLogs(logs []sshmanager.AccessLog, t Theme) string {
	var body strings.Builder
	body.WriteString(t.Heading("SSH access history", "Newest sessions first; timestamps use local time."))
	if len(logs) == 0 {
		body.WriteString(t.Muted("No SSH access logs found."))
		return body.String()
	}
	for _, entry := range logs {
		status := t.Warn(entry.Status)
		switch entry.Status {
		case sshmanager.StatusSuccess:
			status = t.Good(entry.Status)
		case sshmanager.StatusFailed:
			status = t.Accent(entry.Status)
		}
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
		fmt.Fprintf(&body, "  %s · duration %s · exit %s\n\n", t.Muted(entry.StartedAt.Local().Format("2006-01-02 15:04:05 MST")), duration, exit)
	}
	return strings.TrimSuffix(body.String(), "\n")
}

func SSHFailure(err error, t Theme) string {
	message := "SSH management is unavailable."
	if err != nil {
		message = err.Error()
	}
	return t.Heading("SSH", "Local server management") + t.Warn(message)
}

func KeyList(keys []sshmanager.Key, query, notice string, cursor int, t Theme) string {
	subtitle := "Managed keys and key pairs discovered in ~/.ssh."
	if query != "" {
		subtitle = "Results for " + strconv.Quote(query)
	}
	var body strings.Builder
	body.WriteString(t.Heading("SSH keys", subtitle))
	if notice != "" {
		body.WriteString(t.Good(notice) + "\n\n")
	}
	if len(keys) == 0 {
		body.WriteString(t.Muted("No matching SSH key pairs found.\n\n"))
		body.WriteString("Generate one from the TUI command bar:\n\n")
		body.WriteString(t.Accent("/keygen my-key"))
		return body.String()
	}
	for i, key := range keys {
		source := "standard"
		if key.Managed {
			source = "managed"
		}
		description := strings.TrimPrefix(key.Type, "ssh-") + " · " + source + " · " + key.Fingerprint
		body.WriteString(t.Choice(key.Name, description, i == cursor))
	}
	return strings.TrimSuffix(body.String(), "\n")
}

func KeyDetail(key sshmanager.Key, notice string, t Theme) string {
	source := "Standard ~/.ssh key"
	if key.Managed {
		source = "AIAI managed key"
	}
	body := t.Heading(key.Name, source)
	if notice != "" {
		body += t.Good(notice) + "\n\n"
	}
	body += "Type: " + key.Type + "\n" +
		"Fingerprint: " + key.Fingerprint + "\n" +
		"Private key: " + key.PrivatePath + "\n" +
		"Public key: " + key.PublicPath + "\n"
	if key.Comment != "" {
		body += "Comment: " + key.Comment + "\n"
	}
	body += "\n" + t.Accent("c") + "  Copy public key\n"
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
