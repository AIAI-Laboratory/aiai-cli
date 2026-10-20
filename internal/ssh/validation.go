package ssh

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	aliasPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	usernamePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	hostLabelPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
)

func NormalizeAlias(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !aliasPattern.MatchString(value) {
		return "", fmt.Errorf("%w: alias must start with a letter or digit and contain only letters, digits, '.', '_' or '-' (maximum 64 characters)", ErrValidation)
	}
	return value, nil
}

func NormalizeHost(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	}
	if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, "@/\\ \t\r\n") {
		return "", fmt.Errorf("%w: host must be a hostname or IP address", ErrValidation)
	}
	if addr, err := netip.ParseAddr(value); err == nil {
		return addr.String(), nil
	}
	if len(value) > 253 {
		return "", fmt.Errorf("%w: hostname exceeds 253 characters", ErrValidation)
	}
	hostname := strings.TrimSuffix(strings.ToLower(value), ".")
	labels := strings.Split(hostname, ".")
	for _, label := range labels {
		if !hostLabelPattern.MatchString(label) {
			return "", fmt.Errorf("%w: host must be a valid ASCII hostname or IP address", ErrValidation)
		}
	}
	return hostname, nil
}

func ValidateUsername(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !usernamePattern.MatchString(value) {
		return "", fmt.Errorf("%w: username must start with a letter or digit and contain only letters, digits, '.', '_' or '-' (maximum 64 characters)", ErrValidation)
	}
	return value, nil
}

func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%w: port must be between 1 and 65535", ErrValidation)
	}
	return nil
}

func ValidateDescription(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len([]rune(value)) > 500 {
		return "", fmt.Errorf("%w: description must not exceed 500 characters", ErrValidation)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: description must not contain control characters", ErrValidation)
		}
	}
	return value, nil
}

func NormalizeKeyPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: key path is required", ErrValidation)
	}
	if value == "~" || strings.HasPrefix(value, "~/") || strings.HasPrefix(value, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("%w: locate home directory: %v", ErrValidation, err)
		}
		if value == "~" {
			value = home
		} else {
			value = filepath.Join(home, value[2:])
		}
	} else if strings.HasPrefix(value, "~") {
		return "", fmt.Errorf("%w: only '~/' home expansion is supported", ErrValidation)
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("%w: normalize key path: %v", ErrValidation, err)
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("%w: key file %q is unavailable: %v", ErrValidation, abs, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: key path %q is not a regular file", ErrValidation, abs)
	}
	file, err := os.Open(abs)
	if err != nil {
		return "", fmt.Errorf("%w: key file %q is not readable: %v", ErrValidation, abs, err)
	}
	_ = file.Close()
	return abs, nil
}

func validateStoredServer(server Server, revalidateKey bool) (Server, error) {
	alias, err := NormalizeAlias(server.Alias)
	if err != nil {
		return Server{}, err
	}
	host, err := NormalizeHost(server.Host)
	if err != nil {
		return Server{}, err
	}
	username, err := ValidateUsername(server.Username)
	if err != nil {
		return Server{}, err
	}
	if err := ValidatePort(server.Port); err != nil {
		return Server{}, err
	}
	description, err := ValidateDescription(server.Description)
	if err != nil {
		return Server{}, err
	}
	server.Alias, server.Host, server.Username, server.Description = alias, host, username, description
	if revalidateKey && server.IdentityFile != "" {
		server.IdentityFile, err = NormalizeKeyPath(server.IdentityFile)
		if err != nil {
			return Server{}, err
		}
	}
	return server, nil
}
