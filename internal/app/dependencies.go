package app

import (
	"io"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	"github.com/AIAI-Laboratory/aiai-cli/internal/command"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
	"github.com/AIAI-Laboratory/aiai-cli/internal/updater"
)

func dependencies(in io.Reader, out, errOut io.Writer, version, injectedAPIURL, injectedGitHubClientID string) (command.Dependencies, error) {
	r, err := templates.Embedded()
	if err != nil {
		return command.Dependencies{}, err
	}
	store, err := auth.NewCredentialStore("")
	var credentialStore auth.Store = store
	if err != nil {
		credentialStore = auth.FailingStore{Err: err}
	}
	apiURL := auth.ResolveAPIURL(injectedAPIURL)
	clientID := auth.ResolveGitHubClientID(injectedGitHubClientID)
	client := auth.Client{
		ClientID:  clientID,
		BaseURL:   apiURL,
		HTTP:      auth.DefaultHTTPClient(),
		UserAgent: "aiai-cli/" + version,
	}
	authService := auth.NewService(client, credentialStore, apiURL)
	updaterService, _ := updater.NewService("", "", "aiai-cli/"+version, "")
	return command.Dependencies{
		Planner:  project.Initializer{Engine: scaffold.Engine{Registry: r}},
		Executor: scaffold.FileExecutor{},
		Registry: r,
		Auth:     authService,
		Updater:  updaterService,
		In:       in,
		Out:      out,
		Err:      errOut,
		Version:  version,
	}, nil
}
