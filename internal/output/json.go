package output

import (
	"encoding/json"
	"io"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
	"github.com/AIAI-Laboratory/aiai-cli/internal/updater"
)

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Auth struct {
	Action          string     `json:"action"`
	Authenticated   bool       `json:"authenticated"`
	User            *auth.User `json:"user,omitempty"`
	CredentialStore string     `json:"credential_store,omitempty"`
	Warning         string     `json:"warning,omitempty"`
}

type Update struct {
	CurrentVersion string           `json:"current_version"`
	LatestVersion  string           `json:"latest_version,omitempty"`
	HasUpdate      bool             `json:"has_update"`
	Action         string           `json:"action"`
	Release        *updater.Release `json:"release,omitempty"`
}

type SSH struct {
	Action  string                  `json:"action"`
	Alias   string                  `json:"alias,omitempty"`
	Server  *sshmanager.Server      `json:"server,omitempty"`
	Servers *[]sshmanager.Server    `json:"servers,omitempty"`
	KeyPath *string                 `json:"key_path,omitempty"`
	Logs    *[]sshmanager.AccessLog `json:"logs,omitempty"`
	Key     *sshmanager.Key         `json:"key,omitempty"`
	Keys    *[]sshmanager.Key       `json:"keys,omitempty"`
}

// Envelope is the versioned machine-output contract. File bodies are omitted.
type Envelope struct {
	SchemaVersion int              `json:"schema_version"`
	Status        string           `json:"status"`
	Plan          *scaffold.Plan   `json:"plan,omitempty"`
	Result        *scaffold.Result `json:"result,omitempty"`
	Error         *Error           `json:"error,omitempty"`
	Version       string           `json:"version,omitempty"`
	Auth          *Auth            `json:"auth,omitempty"`
	Update        *Update          `json:"update,omitempty"`
	SSH           *SSH             `json:"ssh,omitempty"`
}

func JSON(w io.Writer, e Envelope) error {
	e.SchemaVersion = 1
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}
