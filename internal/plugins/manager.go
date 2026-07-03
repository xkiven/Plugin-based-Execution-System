package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"pluginexec/sdk"
)

type Manager struct {
	pluginDir string
	store     *StateStore
}

func NewManager(pluginDir string) *Manager {
	return &Manager{
		pluginDir: pluginDir,
		store:     NewStateStore(pluginDir),
	}
}

func (m *Manager) Discover() ([]Plugin, error) {
	state, err := m.store.Load()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(m.pluginDir)
	if errors.Is(err, os.ErrNotExist) {
		return []Plugin{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read plugin directory: %w", err)
	}

	type discoveredManifest struct {
		dir      string
		manifest Manifest
	}

	seen := map[string]struct{}{}
	loaded := make([]discoveredManifest, 0, len(entries))
	plugins := make([]Plugin, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginDir := filepath.Join(m.pluginDir, entry.Name())
		manifestPath := filepath.Join(pluginDir, "plugin.json")
		manifest, err := readManifest(manifestPath)
		if err != nil {
			plugins = append(plugins, Plugin{
				Name:      entry.Name(),
				Directory: pluginDir,
				Status:    StatusError,
				LastError: err.Error(),
			})
			continue
		}

		if _, exists := seen[manifest.Name]; exists {
			plugins = append(plugins, Plugin{
				Name:      manifest.Name,
				Version:   manifest.Version,
				Directory: pluginDir,
				Status:    StatusError,
				LastError: "duplicate plugin name",
			})
			continue
		}
		seen[manifest.Name] = struct{}{}

		loaded = append(loaded, discoveredManifest{
			dir:      pluginDir,
			manifest: manifest,
		})
	}

	index := map[string]*Plugin{}
	for _, item := range loaded {
		manifest := item.manifest

		enabled := true
		if manifest.Enabled != nil {
			enabled = *manifest.Enabled
		}
		if override, ok := state.Overrides[manifest.Name]; ok {
			enabled = override
		}

		status := StatusEnabled
		if !enabled {
			status = StatusDisabled
		}

		lastRun := state.LastRuns[manifest.Name]
		if status == StatusEnabled && lastRun.Status == string(StatusError) {
			status = StatusError
		}

		plugin := Plugin{
			Name:                   manifest.Name,
			Version:                manifest.Version,
			APIVersion:             manifest.APIVersion,
			Description:            manifest.Description,
			Entry:                  filepath.Join(item.dir, manifest.Entry),
			Directory:              item.dir,
			Enabled:                enabled,
			Status:                 status,
			LastError:              lastRun.Error,
			UpdatedAt:              lastRun.UpdatedAt,
			Runtime:                manifest.Runtime,
			Dependencies:           manifest.Dependencies,
			FailurePolicy:          manifest.FailurePolicy,
			CompatibleHostVersions: manifest.CompatibleHostVersions,
		}
		if err := validatePlugin(plugin); err != nil {
			plugin.Status = StatusError
			plugin.LastError = err.Error()
		}

		plugins = append(plugins, plugin)
		index[plugin.Name] = &plugins[len(plugins)-1]
	}

	for i := range plugins {
		plugin := &plugins[i]
		if plugin.Status != StatusEnabled {
			continue
		}
		if err := validateDependencies(*plugin, index); err != nil {
			plugin.Status = StatusError
			plugin.LastError = err.Error()
		}
	}

	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Name < plugins[j].Name
	})

	return plugins, nil
}

func (m *Manager) Enable(name string) error {
	return m.setEnabled(name, true)
}

func (m *Manager) Disable(name string) error {
	return m.setEnabled(name, false)
}

func (m *Manager) setEnabled(name string, enabled bool) error {
	plugins, err := m.Discover()
	if err != nil {
		return err
	}

	found := false
	for _, plugin := range plugins {
		if plugin.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("plugin %q not found", name)
	}

	state, err := m.store.Load()
	if err != nil {
		return err
	}
	state.Overrides[name] = enabled

	return m.store.Save(state)
}

func (m *Manager) ExecuteAll(ctx context.Context, input map[string]any, timeout time.Duration) (ExecutionSummary, error) {
	plugins, err := m.Discover()
	if err != nil {
		return ExecutionSummary{}, err
	}

	startedAt := time.Now()
	var enabledPlugins []Plugin
	for _, plugin := range plugins {
		if plugin.Enabled && plugin.Status == StatusEnabled {
			enabledPlugins = append(enabledPlugins, plugin)
		}
	}

	results := make([]ExecutionResult, len(enabledPlugins))
	var wg sync.WaitGroup
	for i, plugin := range enabledPlugins {
		wg.Add(1)
		go func(idx int, plugin Plugin) {
			defer wg.Done()
			results[idx] = executePlugin(ctx, plugin, input, timeout)
		}(i, plugin)
	}
	wg.Wait()

	if err := m.persistRunResults(results); err != nil {
		return ExecutionSummary{}, err
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Plugin < results[j].Plugin
	})

	return ExecutionSummary{
		StartedAt:   startedAt,
		CompletedAt: time.Now(),
		Input:       input,
		Results:     results,
	}, nil
}

func (m *Manager) persistRunResults(results []ExecutionResult) error {
	state, err := m.store.Load()
	if err != nil {
		return err
	}

	for _, result := range results {
		state.LastRuns[result.Plugin] = RunInfo{
			Status:    result.Status,
			Error:     result.Error,
			UpdatedAt: result.CompletedAt,
		}
	}

	return m.store.Save(state)
}

func readManifest(path string) (Manifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}

	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest %s: %w", path, err)
	}

	return manifest, nil
}

func validatePlugin(plugin Plugin) error {
	if strings.TrimSpace(plugin.Name) == "" {
		return errors.New("plugin name is required")
	}
	if strings.TrimSpace(plugin.Version) == "" {
		return errors.New("plugin version is required")
	}
	if !isValidVersion(plugin.Version) {
		return fmt.Errorf("plugin version %q is invalid", plugin.Version)
	}
	if strings.TrimSpace(plugin.Entry) == "" {
		return errors.New("plugin entry is required")
	}
	if plugin.APIVersion != "" && plugin.APIVersion != HostAPIVersion {
		return fmt.Errorf("unsupported api version %q, host expects %q", plugin.APIVersion, HostAPIVersion)
	}
	if len(plugin.CompatibleHostVersions) > 0 && !contains(plugin.CompatibleHostVersions, HostAPIVersion) {
		return fmt.Errorf("host api version %q not in compatible host versions", HostAPIVersion)
	}
	if err := validateRuntime(plugin.Runtime); err != nil {
		return err
	}
	for _, dependency := range plugin.Dependencies {
		if strings.TrimSpace(dependency.Name) == "" {
			return errors.New("dependency name is required")
		}
		if dependency.MinVersion != "" && !isValidVersion(dependency.MinVersion) {
			return fmt.Errorf("dependency %q has invalid min_version %q", dependency.Name, dependency.MinVersion)
		}
	}
	info, err := os.Stat(plugin.Entry)
	if err != nil {
		return fmt.Errorf("plugin entry not found: %w", err)
	}
	if info.IsDir() {
		return errors.New("plugin entry must be a file")
	}
	return nil
}

func validateRuntime(runtime RuntimeSpec) error {
	runtimeType := runtime.Type
	if runtimeType == "" {
		runtimeType = "executable"
	}

	switch runtimeType {
	case "executable":
		return nil
	case "command":
		if strings.TrimSpace(runtime.Command) == "" {
			return errors.New("runtime.command is required when runtime.type is command")
		}
		return nil
	default:
		return fmt.Errorf("unsupported runtime type %q", runtime.Type)
	}
}

func validateDependencies(plugin Plugin, index map[string]*Plugin) error {
	for _, dependency := range plugin.Dependencies {
		dependent, ok := index[dependency.Name]
		if !ok {
			return fmt.Errorf("missing dependency %q", dependency.Name)
		}
		if !dependent.Enabled || dependent.Status != StatusEnabled {
			return fmt.Errorf("dependency %q is not available", dependency.Name)
		}
		if dependency.MinVersion != "" && compareVersions(dependent.Version, dependency.MinVersion) < 0 {
			return fmt.Errorf("dependency %q version %s does not satisfy min_version %s", dependency.Name, dependent.Version, dependency.MinVersion)
		}
	}
	return nil
}

func executePlugin(parent context.Context, plugin Plugin, input map[string]any, timeout time.Duration) ExecutionResult {
	startedAt := time.Now()
	result := ExecutionResult{
		Plugin:    plugin.Name,
		Version:   plugin.Version,
		StartedAt: startedAt,
		Status:    "success",
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	payload, err := json.Marshal(sdk.ExecuteRequest{Data: input})
	if err != nil {
		result.Status = string(StatusError)
		result.Error = fmt.Sprintf("marshal request: %v", err)
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(startedAt)
		return result
	}

	command, args := buildCommand(plugin)
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = plugin.Directory
	cmd.Stdin = strings.NewReader(string(payload))
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(startedAt)

	if ctx.Err() == context.DeadlineExceeded {
		result.Status = string(StatusError)
		result.Error = fmt.Sprintf("timeout after %s", timeout)
		return applyFailurePolicy(result, plugin)
	}
	if runErr != nil {
		result.Status = string(StatusError)
		result.Error = strings.TrimSpace(stderr.String())
		if result.Error == "" {
			result.Error = runErr.Error()
		}
		return applyFailurePolicy(result, plugin)
	}

	var response sdk.ExecuteResponse
	if err := json.Unmarshal([]byte(stdout.String()), &response); err != nil {
		result.Status = string(StatusError)
		result.Error = fmt.Sprintf("decode plugin response: %v", err)
		return applyFailurePolicy(result, plugin)
	}
	if response.Error != "" {
		result.Status = string(StatusError)
		result.Error = response.Error
		return applyFailurePolicy(result, plugin)
	}

	result.Output = response.Output
	return result
}

func buildCommand(plugin Plugin) (string, []string) {
	runtimeType := plugin.Runtime.Type
	if runtimeType == "" || runtimeType == "executable" {
		return plugin.Entry, nil
	}

	if runtimeType == "command" {
		args := append([]string{}, plugin.Runtime.Args...)
		args = append(args, plugin.Entry)
		return plugin.Runtime.Command, args
	}

	return plugin.Entry, nil
}

func applyFailurePolicy(result ExecutionResult, plugin Plugin) ExecutionResult {
	if !plugin.FailurePolicy.UseFallback {
		return result
	}
	result.Status = "degraded"
	result.Output = plugin.FailurePolicy.FallbackOutput
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isValidVersion(value string) bool {
	return len(parseVersion(value)) > 0
}

func compareVersions(left, right string) int {
	leftParts := parseVersion(left)
	rightParts := parseVersion(right)
	maxLen := len(leftParts)
	if len(rightParts) > maxLen {
		maxLen = len(rightParts)
	}

	for i := 0; i < maxLen; i++ {
		var leftValue int
		if i < len(leftParts) {
			leftValue = leftParts[i]
		}
		var rightValue int
		if i < len(rightParts) {
			rightValue = rightParts[i]
		}
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}

	return 0
}

func parseVersion(value string) []int {
	chunks := strings.Split(strings.TrimSpace(value), ".")
	parts := make([]int, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk == "" {
			return nil
		}
		item, err := strconv.Atoi(chunk)
		if err != nil {
			return nil
		}
		parts = append(parts, item)
	}
	return parts
}
