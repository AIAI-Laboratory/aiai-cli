package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type StateStore interface {
	Load() (State, error)
	Save(state State) error
}

type FileStateStore struct {
	FilePath string
}

func DefaultStatePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(base, "aiai", "update_state.json"), nil
}

func NewFileStateStore(filePath string) (*FileStateStore, error) {
	if filePath == "" {
		p, err := DefaultStatePath()
		if err != nil {
			return nil, err
		}
		filePath = p
	}
	return &FileStateStore{FilePath: filePath}, nil
}

func (s *FileStateStore) Load() (State, error) {
	data, err := os.ReadFile(s.FilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("read update state file: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, nil
	}
	return state, nil
}

func (s *FileStateStore) Save(state State) error {
	dir := filepath.Dir(s.FilePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory for update state: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal update state: %w", err)
	}

	tempPath := s.FilePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return fmt.Errorf("write update state temp file: %w", err)
	}
	if err := os.Rename(tempPath, s.FilePath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("commit update state file: %w", err)
	}
	return nil
}
