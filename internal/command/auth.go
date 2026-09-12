package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
)

func (s *state) loginCommand() *cobra.Command {
	var force, noBrowser bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to AIAI with a GitHub account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if s.deps.Auth == nil {
				return errors.New("authentication is unavailable")
			}
			result, err := s.deps.Auth.Login(cmd.Context(), auth.LoginOptions{Force: force, NoBrowser: noBrowser}, func(device auth.DeviceAuthorization) {
				fmt.Fprintf(s.human(), "Open %s\nEnter code: %s\n\nWaiting for GitHub authorization…\n", device.VerificationURI, device.UserCode)
			})
			s.setAuthOutput("login", result)
			if err != nil {
				return authCommandError(err)
			}
			if !s.json {
				fmt.Fprintf(s.human(), "Signed in as @%s.\n", result.User.Login)
				printWarning(s.human(), result.Warning)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Replace the current session after a new login succeeds")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Do not try to open the verification URL")
	return cmd
}

func (s *state) whoamiCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the signed-in AIAI account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if s.deps.Auth == nil {
				return errors.New("authentication is unavailable")
			}
			result, err := s.deps.Auth.WhoAmI(cmd.Context())
			s.setAuthOutput("whoami", result)
			if err != nil {
				return authCommandError(err)
			}
			if !s.json {
				printUser(s.human(), result.User)
				printWarning(s.human(), result.Warning)
			}
			return nil
		},
	}
}

func (s *state) logoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Sign out and remove local AIAI credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if s.deps.Auth == nil {
				return errors.New("authentication is unavailable")
			}
			result, err := s.deps.Auth.Logout(cmd.Context())
			s.auth = &output.Auth{Action: "logout", Authenticated: false, Warning: result.Warning}
			if err != nil {
				return authCommandError(err)
			}
			if !s.json {
				fmt.Fprintln(s.human(), "Signed out.")
				printWarning(s.human(), result.Warning)
			}
			return nil
		},
	}
}

func (s *state) setAuthOutput(action string, result auth.Result) {
	value := &output.Auth{Action: action, Authenticated: result.Authenticated, CredentialStore: result.Storage, Warning: result.Warning}
	if result.User.ID != "" {
		user := result.User
		value.User = &user
	}
	s.auth = value
}

func authCommandError(err error) error {
	switch {
	case errors.Is(err, auth.ErrUnauthenticated):
		return fmt.Errorf("not signed in; run aiai login: %w", err)
	case errors.Is(err, auth.ErrDenied):
		return fmt.Errorf("GitHub authorization was denied: %w", err)
	case errors.Is(err, auth.ErrExpired):
		return fmt.Errorf("GitHub authorization expired; run aiai login again: %w", err)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return err
	}
}

func printUser(w interface{ Write([]byte) (int, error) }, user auth.User) {
	fmt.Fprintf(w, "@%s\n", user.Login)
	if user.Name != "" {
		fmt.Fprintf(w, "%s\n", user.Name)
	}
	fmt.Fprintf(w, "GitHub ID: %s\n", user.GitHubID)
}

func printWarning(w interface{ Write([]byte) (int, error) }, warning string) {
	if warning != "" {
		fmt.Fprintf(w, "Warning: %s\n", warning)
	}
}
