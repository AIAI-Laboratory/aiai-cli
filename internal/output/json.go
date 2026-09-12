package output

import (
	"encoding/json"
	"io"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
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

// Envelope is the versioned machine-output contract. File bodies are omitted.
type Envelope struct {
	SchemaVersion int              `json:"schema_version"`
	Status        string           `json:"status"`
	Plan          *scaffold.Plan   `json:"plan,omitempty"`
	Result        *scaffold.Result `json:"result,omitempty"`
	Error         *Error           `json:"error,omitempty"`
	Version       string           `json:"version,omitempty"`
	Auth          *Auth            `json:"auth,omitempty"`
}

func JSON(w io.Writer, e Envelope) error {
	e.SchemaVersion = 1
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}
