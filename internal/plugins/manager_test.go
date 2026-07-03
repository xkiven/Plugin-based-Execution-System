package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pluginexec/sdk"
)

func TestDiscoverHonorsOverride(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(filepath.Join(pluginDir, "echo"), 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}

	manifest := `{
  "name": "echo",
  "version": "1.0.0",
  "entry": "echo.exe",
  "enabled": true
}`
	if err := os.WriteFile(filepath.Join(pluginDir, "echo", "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "echo", "echo.exe"), []byte("binary"), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}

	manager := NewManager(pluginDir)
	state, err := manager.store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	state.Overrides["echo"] = false
	if err := manager.store.Save(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	plugins, err := manager.Discover()
	if err != nil {
		t.Fatalf("discover plugins: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("unexpected plugin count: %d", len(plugins))
	}
	if plugins[0].Enabled {
		t.Fatalf("expected plugin to be disabled by override")
	}
	if plugins[0].Status != StatusDisabled {
		t.Fatalf("unexpected status: %s", plugins[0].Status)
	}
}

func TestDiscoverDependencyValidation(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins")

	mustMkdirAll(t, filepath.Join(pluginDir, "base"))
	mustWriteFile(t, filepath.Join(pluginDir, "base", "base.exe"), []byte("binary"))
	mustWriteFile(t, filepath.Join(pluginDir, "base", "plugin.json"), []byte(`{
  "name": "base",
  "version": "1.0.0",
  "entry": "base.exe",
  "enabled": false
}`))

	mustMkdirAll(t, filepath.Join(pluginDir, "dependent"))
	mustWriteFile(t, filepath.Join(pluginDir, "dependent", "dependent.exe"), []byte("binary"))
	mustWriteFile(t, filepath.Join(pluginDir, "dependent", "plugin.json"), []byte(`{
  "name": "dependent",
  "version": "1.0.0",
  "entry": "dependent.exe",
  "dependencies": [
    {
      "name": "base",
      "min_version": "1.0.0"
    }
  ]
}`))

	manager := NewManager(pluginDir)
	plugins, err := manager.Discover()
	if err != nil {
		t.Fatalf("discover plugins: %v", err)
	}

	byName := map[string]Plugin{}
	for _, plugin := range plugins {
		byName[plugin.Name] = plugin
	}
	if byName["dependent"].Status != StatusError {
		t.Fatalf("expected dependent plugin status error, got %s", byName["dependent"].Status)
	}
}

func TestApplyFailurePolicy(t *testing.T) {
	result := applyFailurePolicy(ExecutionResult{
		Plugin: "unstable",
		Status: string(StatusError),
		Error:  "boom",
	}, Plugin{
		FailurePolicy: FailurePolicy{
			UseFallback: true,
			FallbackOutput: map[string]any{
				"mode": "fallback",
			},
		},
	})

	if result.Status != "degraded" {
		t.Fatalf("expected degraded status, got %s", result.Status)
	}
	if result.Output["mode"] != "fallback" {
		t.Fatalf("unexpected fallback output: %#v", result.Output)
	}
}

func TestExecuteAllPersistsResults(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins")

	mustCreateHelperPlugin(t, pluginDir, "zeta", "success.case", FailurePolicy{})
	mustCreateHelperPlugin(t, pluginDir, "alpha", "sleep.case", FailurePolicy{
		UseFallback: true,
		FallbackOutput: map[string]any{
			"mode": "fallback",
		},
	})

	manager := NewManager(pluginDir)
	summary, err := manager.ExecuteAll(context.Background(), map[string]any{
		"message": "hello",
	}, time.Second)
	if err != nil {
		t.Fatalf("execute all: %v", err)
	}

	if len(summary.Results) != 2 {
		t.Fatalf("unexpected result count: %d", len(summary.Results))
	}

	if summary.Results[0].Plugin != "alpha" || summary.Results[0].Status != "degraded" {
		t.Fatalf("unexpected first result: %#v", summary.Results[0])
	}
	if summary.Results[0].Output["mode"] != "fallback" {
		t.Fatalf("unexpected fallback output: %#v", summary.Results[0].Output)
	}

	if summary.Results[1].Plugin != "zeta" || summary.Results[1].Status != "success" {
		t.Fatalf("unexpected second result: %#v", summary.Results[1])
	}
	if summary.Results[1].Output["message"] != "hello" {
		t.Fatalf("unexpected success output: %#v", summary.Results[1].Output)
	}

	state, err := manager.store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.LastRuns["alpha"].Status != "degraded" {
		t.Fatalf("expected alpha degraded, got %#v", state.LastRuns["alpha"])
	}
	if state.LastRuns["zeta"].Status != "success" {
		t.Fatalf("expected zeta success, got %#v", state.LastRuns["zeta"])
	}
	if state.LastRuns["alpha"].UpdatedAt.IsZero() || state.LastRuns["zeta"].UpdatedAt.IsZero() {
		t.Fatalf("expected persisted timestamps, got %#v", state.LastRuns)
	}
}

func TestExecutePluginInvalidJSON(t *testing.T) {
	plugin := newHelperPlugin(t, t.TempDir(), "invalid-json", "invalid-json.case", FailurePolicy{})

	result := executePlugin(context.Background(), plugin, map[string]any{
		"message": "hello",
	}, time.Second)

	if result.Status != string(StatusError) {
		t.Fatalf("expected error status, got %#v", result)
	}
	if !strings.Contains(result.Error, "decode plugin response") {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

func TestExecutePluginPropagatesPluginError(t *testing.T) {
	plugin := newHelperPlugin(t, t.TempDir(), "plugin-error", "plugin-error.case", FailurePolicy{})

	result := executePlugin(context.Background(), plugin, map[string]any{}, time.Second)

	if result.Status != string(StatusError) {
		t.Fatalf("expected error status, got %#v", result)
	}
	if result.Error != "plugin boom" {
		t.Fatalf("unexpected plugin error: %#v", result)
	}
}

func TestExecutePluginFallsBackToRunErrorWhenStderrEmpty(t *testing.T) {
	plugin := newHelperPlugin(t, t.TempDir(), "silent-fail", "silent-fail.case", FailurePolicy{})

	result := executePlugin(context.Background(), plugin, map[string]any{}, time.Second)

	if result.Status != string(StatusError) {
		t.Fatalf("expected error status, got %#v", result)
	}
	if result.Error == "" || !strings.Contains(result.Error, "exit status") {
		t.Fatalf("expected process exit error, got %#v", result)
	}
}

func TestValidatePlugin(t *testing.T) {
	tempDir := t.TempDir()
	entryFile := filepath.Join(tempDir, "entry.bin")
	mustWriteFile(t, entryFile, []byte("binary"))

	tests := []struct {
		name      string
		plugin    Plugin
		wantError string
	}{
		{
			name: "invalid version",
			plugin: Plugin{
				Name:    "sample",
				Version: "1.a.0",
				Entry:   entryFile,
			},
			wantError: `plugin version "1.a.0" is invalid`,
		},
		{
			name: "unsupported api version",
			plugin: Plugin{
				Name:       "sample",
				Version:    "1.0.0",
				Entry:      entryFile,
				APIVersion: "2.0.0",
			},
			wantError: `unsupported api version "2.0.0"`,
		},
		{
			name: "incompatible host version",
			plugin: Plugin{
				Name:                   "sample",
				Version:                "1.0.0",
				Entry:                  entryFile,
				CompatibleHostVersions: []string{"9.9.9"},
			},
			wantError: `host api version "1.0.0" not in compatible host versions`,
		},
		{
			name: "command runtime missing command",
			plugin: Plugin{
				Name:    "sample",
				Version: "1.0.0",
				Entry:   entryFile,
				Runtime: RuntimeSpec{Type: "command"},
			},
			wantError: "runtime.command is required when runtime.type is command",
		},
		{
			name: "invalid dependency min version",
			plugin: Plugin{
				Name:    "sample",
				Version: "1.0.0",
				Entry:   entryFile,
				Dependencies: []Dependency{{
					Name:       "base",
					MinVersion: "1.x",
				}},
			},
			wantError: `dependency "base" has invalid min_version "1.x"`,
		},
		{
			name: "entry is directory",
			plugin: Plugin{
				Name:    "sample",
				Version: "1.0.0",
				Entry:   tempDir,
			},
			wantError: "plugin entry must be a file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePlugin(tt.plugin)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("expected %q in error %q", tt.wantError, err.Error())
			}
		})
	}
}

func TestPluginHelperProcess(t *testing.T) {
	entry, ok := helperEntryArg(os.Args)
	if !ok {
		return
	}

	request := helperRequest(t)

	switch filepath.Base(entry) {
	case "success.case":
		encodeHelperResponse(t, sdk.ExecuteResponse{
			Output: map[string]any{
				"message":  request.Data["message"],
				"scenario": "success",
			},
		})
	case "sleep.case":
		time.Sleep(2 * time.Second)
		encodeHelperResponse(t, sdk.ExecuteResponse{
			Output: map[string]any{
				"scenario": "sleep",
			},
		})
	case "invalid-json.case":
		_, _ = fmt.Fprint(os.Stdout, "{invalid json")
	case "plugin-error.case":
		encodeHelperResponse(t, sdk.ExecuteResponse{
			Error: "plugin boom",
		})
	case "silent-fail.case":
		os.Exit(2)
	default:
		t.Fatalf("unknown helper scenario %q", entry)
	}

	os.Exit(0)
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func mustCreateHelperPlugin(t *testing.T, pluginDir, name, entry string, failurePolicy FailurePolicy) {
	t.Helper()

	plugin := newHelperPlugin(t, pluginDir, name, entry, failurePolicy)
	enabled := true
	manifest := Manifest{
		Name:          plugin.Name,
		Version:       plugin.Version,
		Entry:         entry,
		Enabled:       &enabled,
		Runtime:       plugin.Runtime,
		FailurePolicy: failurePolicy,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	mustWriteFile(t, filepath.Join(plugin.Directory, "plugin.json"), body)
}

func newHelperPlugin(t *testing.T, pluginRoot, name, entry string, failurePolicy FailurePolicy) Plugin {
	t.Helper()

	pluginDir := filepath.Join(pluginRoot, name)
	mustMkdirAll(t, pluginDir)
	mustWriteFile(t, filepath.Join(pluginDir, entry), []byte("fixture"))

	return Plugin{
		Name:      name,
		Version:   "1.0.0",
		Entry:     filepath.Join(pluginDir, entry),
		Directory: pluginDir,
		Runtime: RuntimeSpec{
			Type:    "command",
			Command: os.Args[0],
			Args: []string{
				"-test.run=TestPluginHelperProcess",
				"--",
			},
		},
		FailurePolicy: failurePolicy,
	}
}

func helperEntryArg(args []string) (string, bool) {
	for i := 0; i < len(args); i++ {
		if args[i] == "--" && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func helperRequest(t *testing.T) sdk.ExecuteRequest {
	t.Helper()

	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		t.Fatalf("read stdin: %v", err)
	}

	var req sdk.ExecuteRequest
	if len(body) == 0 {
		return req
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return req
}

func encodeHelperResponse(t *testing.T, resp sdk.ExecuteResponse) {
	t.Helper()

	if err := json.NewEncoder(os.Stdout).Encode(resp); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
