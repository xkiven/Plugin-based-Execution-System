package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type StateStore struct {
	path string
}

func NewStateStore(pluginDir string) *StateStore {
	return &StateStore{path: filepath.Join(pluginDir, "state.json")}
}

func (s *StateStore) Load() (State, error) {
	state := State{
		Overrides: map[string]bool{},
		LastRuns:  map[string]RunInfo{},
	}

	body, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read state file: %w", err)
	}

	if err := json.Unmarshal(body, &state); err != nil {
		return State{}, fmt.Errorf("decode state file: %w", err)
	}

	if state.Overrides == nil {
		state.Overrides = map[string]bool{}
	}
	if state.LastRuns == nil {
		state.LastRuns = map[string]RunInfo{}
	}

	return state, nil
}

func (s *StateStore) Save(state State) error {
	if state.Overrides == nil {
		state.Overrides = map[string]bool{}
	}
	if state.LastRuns == nil {
		state.LastRuns = map[string]RunInfo{}
	}

	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state file: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, body, 0o644); err != nil {
		return fmt.Errorf("write temp state file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}

	return nil
}
