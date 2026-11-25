package template

import "github.com/AIAI-Laboratory/aiai-cli/templates"

const (
	// AgentKitID is the default built-in agent setup pack template ID.
	AgentKitID = "agent-kit"
	// PythonMinimalID is the minimal Python 3.13+ project template ID.
	PythonMinimalID = "python-minimal"
)

// Embedded loads and validates all templates compiled into the binary.
func Embedded() (*Registry, error) {
	return NewRegistry(templates.Files, AgentKitID, PythonMinimalID)
}
