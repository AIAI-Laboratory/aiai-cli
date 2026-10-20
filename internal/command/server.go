package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
)

func (s *state) serverCommand() *cobra.Command {
	server := &cobra.Command{Use: "server", Short: "Manage local SSH server profiles", Args: cobra.NoArgs}
	server.AddCommand(
		s.serverAddCommand(),
		s.serverListCommand(),
		s.serverShowCommand(),
		s.serverUpdateCommand(),
		s.serverRemoveCommand(),
		s.serverKeyCommand(),
		s.serverLogsCommand(),
	)
	return server
}

func (s *state) serverAddCommand() *cobra.Command {
	var host, username, key, description string
	var port int
	cmd := &cobra.Command{
		Use:   "add <alias>",
		Short: "Add an SSH server profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := s.requireSSH(); err != nil {
				return err
			}
			server, err := s.deps.SSH.Add(cmd.Context(), sshmanager.AddRequest{
				Alias: args[0], Host: host, Port: port, Username: username, IdentityFile: key, Description: description,
			})
			if err != nil {
				return err
			}
			s.ssh = &output.SSH{Action: "server-add", Alias: server.Alias, Server: &server}
			if !s.json {
				fmt.Fprintf(s.human(), "Added SSH server %q.\n", server.Alias)
				printServer(s.human(), server)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&host, "host", "", "Hostname or IP address (required)")
	cmd.Flags().StringVar(&username, "user", "", "SSH username (required)")
	cmd.Flags().IntVar(&port, "port", 22, "SSH port")
	cmd.Flags().StringVar(&key, "key", "", "Existing private key file path")
	cmd.Flags().StringVar(&description, "description", "", "Optional description")
	_ = cmd.MarkFlagRequired("host")
	_ = cmd.MarkFlagRequired("user")
	return cmd
}

func (s *state) serverListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List SSH server profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := s.requireSSH(); err != nil {
				return err
			}
			servers, err := s.deps.SSH.List(cmd.Context())
			if err != nil {
				return err
			}
			s.ssh = &output.SSH{Action: "server-list", Servers: &servers}
			if !s.json {
				printServers(s.human(), servers)
			}
			return nil
		},
	}
}

func (s *state) serverShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <alias>",
		Short: "Show an SSH server profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := s.requireSSH(); err != nil {
				return err
			}
			server, err := s.deps.SSH.Show(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			s.ssh = &output.SSH{Action: "server-show", Alias: server.Alias, Server: &server}
			if !s.json {
				printServer(s.human(), server)
			}
			return nil
		},
	}
}

func (s *state) serverUpdateCommand() *cobra.Command {
	var host, username, key, description string
	var port int
	cmd := &cobra.Command{
		Use:   "update <alias>",
		Short: "Update selected fields of an SSH server profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := s.requireSSH(); err != nil {
				return err
			}
			request := sshmanager.UpdateRequest{}
			if cmd.Flags().Changed("host") {
				request.Host = &host
			}
			if cmd.Flags().Changed("user") {
				request.Username = &username
			}
			if cmd.Flags().Changed("port") {
				request.Port = &port
			}
			if cmd.Flags().Changed("key") {
				request.IdentityFile = &key
			}
			if cmd.Flags().Changed("description") {
				request.Description = &description
			}
			if request.Host == nil && request.Username == nil && request.Port == nil && request.IdentityFile == nil && request.Description == nil {
				return errors.New("specify at least one field to update")
			}
			server, err := s.deps.SSH.Update(cmd.Context(), args[0], request)
			if err != nil {
				return err
			}
			s.ssh = &output.SSH{Action: "server-update", Alias: server.Alias, Server: &server}
			if !s.json {
				fmt.Fprintf(s.human(), "Updated SSH server %q.\n", server.Alias)
				printServer(s.human(), server)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&host, "host", "", "New hostname or IP address")
	cmd.Flags().StringVar(&username, "user", "", "New SSH username")
	cmd.Flags().IntVar(&port, "port", 0, "New SSH port")
	cmd.Flags().StringVar(&key, "key", "", "New private key path; use 'server key clear' to unset")
	cmd.Flags().StringVar(&description, "description", "", "New description; pass an empty value to clear")
	return cmd
}

func (s *state) serverRemoveCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <alias>",
		Short: "Remove an SSH server profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := s.requireSSH(); err != nil {
				return err
			}
			server, err := s.deps.SSH.Show(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if !yes {
				if !s.interactive() {
					return errors.New("non-interactive removal requires --yes")
				}
				fmt.Fprintf(s.human(), "Remove SSH server %q? Its access logs will be kept. [y/N] ", server.Alias)
				approved, err := confirm(cmd.Context(), s.deps.In)
				if err != nil {
					return err
				}
				if !approved {
					return contextCanceled()
				}
			}
			if err := s.deps.SSH.Remove(cmd.Context(), server.Alias); err != nil {
				return err
			}
			s.ssh = &output.SSH{Action: "server-remove", Alias: server.Alias}
			if !s.json {
				fmt.Fprintf(s.human(), "Removed SSH server %q. Access logs were preserved.\n", server.Alias)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Remove without prompting")
	return cmd
}

func (s *state) serverKeyCommand() *cobra.Command {
	key := &cobra.Command{Use: "key", Short: "Manage an SSH private key reference", Args: cobra.NoArgs}
	key.AddCommand(
		&cobra.Command{Use: "set <alias> <key-path>", Short: "Set an existing private key reference", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
			if err := s.requireSSH(); err != nil {
				return err
			}
			server, err := s.deps.SSH.SetKey(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			path := server.IdentityFile
			s.ssh = &output.SSH{Action: "key-set", Alias: server.Alias, KeyPath: &path}
			if !s.json {
				fmt.Fprintf(s.human(), "SSH key reference for %q: %s\n", server.Alias, path)
			}
			return nil
		}},
		&cobra.Command{Use: "show <alias>", Short: "Show the configured private key reference", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			if err := s.requireSSH(); err != nil {
				return err
			}
			path, err := s.deps.SSH.ShowKey(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			alias, _ := sshmanager.NormalizeAlias(args[0])
			s.ssh = &output.SSH{Action: "key-show", Alias: alias, KeyPath: &path}
			if !s.json {
				if path == "" {
					fmt.Fprintln(s.human(), "No explicit key is configured; OpenSSH may use ssh-agent or its standard configuration.")
				} else {
					fmt.Fprintln(s.human(), path)
				}
			}
			return nil
		}},
		&cobra.Command{Use: "clear <alias>", Short: "Clear the private key reference", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			if err := s.requireSSH(); err != nil {
				return err
			}
			server, err := s.deps.SSH.ClearKey(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			empty := ""
			s.ssh = &output.SSH{Action: "key-clear", Alias: server.Alias, KeyPath: &empty}
			if !s.json {
				fmt.Fprintf(s.human(), "Cleared the SSH key reference for %q.\n", server.Alias)
			}
			return nil
		}},
	)
	return key
}

func (s *state) serverLogsCommand() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "logs <alias>",
		Short: "List access logs for an SSH server alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.listLogs(cmd, args[0], limit)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of logs (1-1000)")
	return cmd
}

func (s *state) requireSSH() error {
	if s.deps.SSH == nil {
		return errors.New("SSH management is unavailable")
	}
	return nil
}

func printServer(w io.Writer, server sshmanager.Server) {
	fmt.Fprintf(w, "Alias: %s\nHost: %s\nPort: %d\nUser: %s\n", server.Alias, server.Host, server.Port, server.Username)
	if server.IdentityFile != "" {
		fmt.Fprintf(w, "Key: %s\n", server.IdentityFile)
	} else {
		fmt.Fprintln(w, "Key: default OpenSSH/ssh-agent behavior")
	}
	if server.Description != "" {
		fmt.Fprintf(w, "Description: %s\n", server.Description)
	}
	fmt.Fprintf(w, "Created: %s\nUpdated: %s\n", localTime(server.CreatedAt), localTime(server.UpdatedAt))
}

func printServers(w io.Writer, servers []sshmanager.Server) {
	if len(servers) == 0 {
		fmt.Fprintln(w, "No SSH servers configured.")
		return
	}
	fmt.Fprintf(w, "%-20s %-30s %-7s %-20s %s\n", "ALIAS", "HOST", "PORT", "USER", "KEY")
	for _, server := range servers {
		key := "agent/config"
		if server.IdentityFile != "" {
			key = server.IdentityFile
		}
		fmt.Fprintf(w, "%-20s %-30s %-7s %-20s %s\n", server.Alias, server.Host, strconv.Itoa(server.Port), server.Username, key)
	}
}

func localTime(value time.Time) string {
	return value.Local().Format(time.RFC3339)
}

func contextCanceled() error {
	return fmt.Errorf("operation canceled: %w", context.Canceled)
}
