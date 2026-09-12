package output

import (
	"encoding/json"
	"io"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
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
}

func JSON(w io.Writer, e Envelope) error {
	e.SchemaVersion = 1
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}
