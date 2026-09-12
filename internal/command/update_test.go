package command

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
	"github.com/AIAI-Laboratory/aiai-cli/internal/updater"
)

type fakeUpdater struct {
	checkResult *updater.CheckResult
	checkErr    error
	applyErr    error
	postponeErr error
	applied     bool
}

func (f *fakeUpdater) CheckForUpdate(ctx context.Context, currentVersion string, force bool) (*updater.CheckResult, error) {
	if f.checkErr != nil {
		return nil, f.checkErr
	}
	if f.checkResult != nil {
		return f.checkResult, nil
	}
	return &updater.CheckResult{CurrentVersion: currentVersion, HasUpdate: false}, nil
}

func (f *fakeUpdater) ApplyUpdate(ctx context.Context, release updater.Release) error {
	f.applied = true
	return f.applyErr
}

func (f *fakeUpdater) Postpone(release updater.Release, skip bool) error {
	return f.postponeErr
}

func (f *fakeUpdater) Restart() error {
	return nil
}

func runWithUpdater(t *testing.T, args []string, interactive bool, input string, u updater.Service) (int, string, string) {
	t.Helper()
	r, err := templates.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	deps := Dependencies{
		Planner:     project.Initializer{Engine: scaffold.Engine{Registry: r}},
		Executor:    scaffold.FileExecutor{},
		Registry:    r,
		In:          strings.NewReader(input),
		Out:         &out,
		Err:         &stderr,
		Version:     "v0.1.0",
		Updater:     u,
		Interactive: func() bool { return interactive },
	}
	code := Run(context.Background(), args, deps)
	return code, out.String(), stderr.String()
}

func TestUpdateCheckOnlyJSON(t *testing.T) {
	u := &fakeUpdater{
		checkResult: &updater.CheckResult{
			HasUpdate:      true,
			CurrentVersion: "v0.1.0",
			LatestRelease: &updater.Release{
				TagName: "v0.2.0",
				Version: "0.2.0",
				Name:    "v0.2.0",
			},
		},
	}

	code, stdout, stderr := runWithUpdater(t, []string{"update", "--check", "--json"}, false, "", u)
	if code != 0 {
		t.Fatalf("expected code 0, got %d, stderr: %s", code, stderr)
	}

	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}

	if env.Update == nil {
		t.Fatalf("expected Update in envelope, got nil")
	}
	if !env.Update.HasUpdate {
		t.Errorf("expected HasUpdate=true")
	}
	if env.Update.LatestVersion != "v0.2.0" {
		t.Errorf("expected LatestVersion=v0.2.0, got %s", env.Update.LatestVersion)
	}
	if env.Update.Action != "check" {
		t.Errorf("expected Action=check, got %s", env.Update.Action)
	}
}

func TestUpdateApplyJSON(t *testing.T) {
	u := &fakeUpdater{
		checkResult: &updater.CheckResult{
			HasUpdate:      true,
			CurrentVersion: "v0.1.0",
			LatestRelease: &updater.Release{
				TagName: "v0.2.0",
				Version: "0.2.0",
			},
		},
	}

	code, stdout, stderr := runWithUpdater(t, []string{"update", "--json"}, false, "", u)
	if code != 0 {
		t.Fatalf("expected code 0, got %d, stderr: %s", code, stderr)
	}
	if !u.applied {
		t.Errorf("expected ApplyUpdate to be called")
	}

	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}

	if env.Update == nil || env.Update.Action != "updated" {
		t.Errorf("expected Action=updated, got %+v", env.Update)
	}
}

func TestUpdateAlreadyUpToDate(t *testing.T) {
	u := &fakeUpdater{
		checkResult: &updater.CheckResult{
			HasUpdate:      false,
			CurrentVersion: "v0.2.0",
			LatestRelease: &updater.Release{
				TagName: "v0.2.0",
				Version: "0.2.0",
			},
		},
	}

	code, stdout, stderr := runWithUpdater(t, []string{"update"}, false, "", u)
	if code != 0 {
		t.Fatalf("expected code 0, got %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "already up to date") {
		t.Errorf("expected output to contain 'already up to date', got %s", stdout)
	}
	if u.applied {
		t.Errorf("should not apply update if already up to date")
	}
}
