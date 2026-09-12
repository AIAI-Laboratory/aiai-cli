package command

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
)

func (s *state) updateCommand() *cobra.Command {
	var checkOnly, force bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for updates and upgrade AIAI CLI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if s.deps.Updater == nil {
				return errors.New("updater is unavailable")
			}

			checkRes, err := s.deps.Updater.CheckForUpdate(cmd.Context(), s.deps.Version, force)
			if err != nil {
				return fmt.Errorf("check for update: %w", err)
			}

			latestVer := ""
			if checkRes.LatestRelease != nil {
				latestVer = checkRes.LatestRelease.TagName
			}

			if checkOnly {
				s.update = &output.Update{
					CurrentVersion: s.deps.Version,
					LatestVersion:  latestVer,
					HasUpdate:      checkRes.HasUpdate,
					Action:         "check",
					Release:        checkRes.LatestRelease,
				}
				if !s.json {
					if checkRes.HasUpdate && checkRes.LatestRelease != nil {
						fmt.Fprintf(s.human(), "A new version of AIAI CLI is available: %s (current: %s)\nRun 'aiai update' to upgrade.\n", checkRes.LatestRelease.TagName, s.deps.Version)
					} else {
						fmt.Fprintf(s.human(), "AIAI CLI is already up to date (%s).\n", s.deps.Version)
					}
				}
				return nil
			}

			if !checkRes.HasUpdate && !force {
				s.update = &output.Update{
					CurrentVersion: s.deps.Version,
					LatestVersion:  latestVer,
					HasUpdate:      false,
					Action:         "up-to-date",
					Release:        checkRes.LatestRelease,
				}
				if !s.json {
					fmt.Fprintf(s.human(), "AIAI CLI is already up to date (%s).\n", s.deps.Version)
				}
				return nil
			}

			if checkRes.LatestRelease == nil {
				return errors.New("no release available to install")
			}

			if !s.json {
				fmt.Fprintf(s.human(), "Upgrading AIAI CLI from %s to %s…\n", s.deps.Version, checkRes.LatestRelease.TagName)
			}

			if err := s.deps.Updater.ApplyUpdate(cmd.Context(), *checkRes.LatestRelease); err != nil {
				return fmt.Errorf("apply update: %w", err)
			}

			s.update = &output.Update{
				CurrentVersion: s.deps.Version,
				LatestVersion:  checkRes.LatestRelease.Version,
				HasUpdate:      false,
				Action:         "updated",
				Release:        checkRes.LatestRelease,
			}

			if !s.json {
				fmt.Fprintf(s.human(), "Successfully updated AIAI CLI to %s!\n", checkRes.LatestRelease.TagName)
			}

			return nil
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "Check for updates without downloading or installing")
	cmd.Flags().BoolVar(&force, "force", false, "Force download and reinstall the latest release")
	return cmd
}
