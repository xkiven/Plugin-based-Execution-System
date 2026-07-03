package plugins

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStateStoreLoadMissingFile(t *testing.T) {
	store := NewStateStore(t.TempDir())

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load missing file: %v", err)
	}
	if state.Overrides == nil || state.LastRuns == nil {
		t.Fatalf("expected initialized maps, got %#v", state)
	}
	if len(state.Overrides) != 0 || len(state.LastRuns) != 0 {
		t.Fatalf("expected empty state, got %#v", state)
	}
}

func TestStateStoreSaveLoadRoundTrip(t *testing.T) {
	pluginDir := t.TempDir()
	store := NewStateStore(pluginDir)
	now := time.Now().UTC().Round(time.Second)

	if err := store.Save(State{
		Overrides: map[string]bool{
			"echo": true,
		},
		LastRuns: map[string]RunInfo{
			"echo": {
				Status:    "success",
				UpdatedAt: now,
			},
		},
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if !state.Overrides["echo"] {
		t.Fatalf("expected override to persist, got %#v", state)
	}
	if state.LastRuns["echo"].Status != "success" {
		t.Fatalf("expected run status to persist, got %#v", state)
	}
	if !state.LastRuns["echo"].UpdatedAt.Equal(now) {
		t.Fatalf("expected timestamp %s, got %s", now, state.LastRuns["echo"].UpdatedAt)
	}
}

func TestStateStoreSaveInitializesNilMaps(t *testing.T) {
	pluginDir := t.TempDir()
	store := NewStateStore(pluginDir)

	if err := store.Save(State{}); err != nil {
		t.Fatalf("save zero state: %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load zero state: %v", err)
	}
	if state.Overrides == nil || state.LastRuns == nil {
		t.Fatalf("expected initialized maps, got %#v", state)
	}
}

func TestStateStoreLoadInvalidJSON(t *testing.T) {
	pluginDir := t.TempDir()
	mustWriteFile(t, filepath.Join(pluginDir, "state.json"), []byte("{invalid"))

	store := NewStateStore(pluginDir)
	if _, err := store.Load(); err == nil {
		t.Fatalf("expected invalid json error")
	}
}
