package ssh

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

type Service struct {
	Repository Repository
	Runner     Runner
	KeyTool    KeyTool
	KeyDir     string
	Now        func() time.Time
}

func NewService(repository Repository, runner Runner) *Service {
	return &Service{Repository: repository, Runner: runner, KeyTool: OpenSSHKeyTool{}}
}

func (s *Service) Add(ctx context.Context, request AddRequest) (Server, error) {
	server := Server{
		Alias:        request.Alias,
		Host:         request.Host,
		Port:         request.Port,
		Username:     request.Username,
		IdentityFile: request.IdentityFile,
		Description:  request.Description,
	}
	if server.Port == 0 {
		server.Port = 22
	}
	var err error
	server, err = validateStoredServer(server, server.IdentityFile != "")
	if err != nil {
		return Server{}, err
	}
	now := s.now()
	server.CreatedAt, server.UpdatedAt = now, now
	if err := s.Repository.AddServer(ctx, server); err != nil {
		return Server{}, err
	}
	return server, nil
}

func (s *Service) List(ctx context.Context) ([]Server, error) {
	return s.Repository.ListServers(ctx)
}

func (s *Service) Show(ctx context.Context, alias string) (Server, error) {
	alias, err := NormalizeAlias(alias)
	if err != nil {
		return Server{}, err
	}
	return s.Repository.GetServer(ctx, alias)
}

func (s *Service) Update(ctx context.Context, alias string, request UpdateRequest) (Server, error) {
	server, err := s.Show(ctx, alias)
	if err != nil {
		return Server{}, err
	}
	if request.Host != nil {
		server.Host = *request.Host
	}
	if request.Port != nil {
		server.Port = *request.Port
	}
	if request.Username != nil {
		server.Username = *request.Username
	}
	if request.IdentityFile != nil {
		server.IdentityFile = *request.IdentityFile
	}
	if request.Description != nil {
		server.Description = *request.Description
	}
	server, err = validateStoredServer(server, server.IdentityFile != "")
	if err != nil {
		return Server{}, err
	}
	server.UpdatedAt = s.now()
	if err := s.Repository.UpdateServer(ctx, server); err != nil {
		return Server{}, err
	}
	return server, nil
}

func (s *Service) Remove(ctx context.Context, alias string) error {
	alias, err := NormalizeAlias(alias)
	if err != nil {
		return err
	}
	return s.Repository.RemoveServer(ctx, alias)
}

func (s *Service) SetKey(ctx context.Context, alias, keyPath string) (Server, error) {
	return s.Update(ctx, alias, UpdateRequest{IdentityFile: &keyPath})
}

func (s *Service) ShowKey(ctx context.Context, alias string) (string, error) {
	server, err := s.Show(ctx, alias)
	if err != nil {
		return "", err
	}
	return server.IdentityFile, nil
}

func (s *Service) ClearKey(ctx context.Context, alias string) (Server, error) {
	empty := ""
	return s.Update(ctx, alias, UpdateRequest{IdentityFile: &empty})
}

func (s *Service) Connect(ctx context.Context, alias string, streams Streams) error {
	server, err := s.Show(ctx, alias)
	if err != nil {
		return err
	}
	started := s.now()
	entry := AccessLog{
		ID:          rand.Text(),
		ServerAlias: server.Alias,
		Host:        server.Host,
		Username:    server.Username,
		StartedAt:   started,
		Status:      StatusPending,
	}
	if err := s.Repository.StartLog(ctx, entry); err != nil {
		return err
	}

	server, err = validateStoredServer(server, true)
	if err != nil {
		return s.finish(ctx, entry, StatusFailed, nil, err)
	}
	if s.Runner == nil {
		return s.finish(ctx, entry, StatusFailed, nil, fmt.Errorf("%w: runner is unavailable", ErrSSHMissing))
	}
	result, runErr := s.Runner.Run(ctx, BuildArgs(server), streams)
	if runErr != nil {
		status := StatusFailed
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) || ctx.Err() != nil {
			status = StatusCanceled
		}
		return s.finish(ctx, entry, status, nil, runErr)
	}
	exitCode := result.ExitCode
	if exitCode == 0 {
		return s.finish(ctx, entry, StatusSuccess, &exitCode, nil)
	}
	return s.finish(ctx, entry, StatusFailed, &exitCode, ExitError{Code: exitCode})
}

func (s *Service) Logs(ctx context.Context, filter LogFilter) ([]AccessLog, error) {
	if filter.ServerAlias != "" {
		alias, err := NormalizeAlias(filter.ServerAlias)
		if err != nil {
			return nil, err
		}
		filter.ServerAlias = alias
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Limit < 1 || filter.Limit > 1000 {
		return nil, fmt.Errorf("%w: log limit must be between 1 and 1000", ErrValidation)
	}
	return s.Repository.ListLogs(ctx, filter)
}

func (s *Service) finish(ctx context.Context, entry AccessLog, status string, exitCode *int, resultErr error) error {
	ended := s.now()
	duration := ended.Sub(entry.StartedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	entry.EndedAt = &ended
	entry.DurationMS = &duration
	entry.ExitCode = exitCode
	entry.Status = status
	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.Repository.FinishLog(finalizeCtx, entry); err != nil {
		if resultErr != nil {
			return fmt.Errorf("%v; finalize SSH access log: %w", resultErr, err)
		}
		return err
	}
	return resultErr
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
