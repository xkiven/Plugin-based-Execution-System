package samplebuild

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"pluginexec/internal/plugins"
)

type SamplePlugin struct {
	Name        string
	Version     string
	Description string
	SourceDir   string
}

func BuildSamples(projectRoot, pluginDir string) error {
	samples := []SamplePlugin{
		{
			Name:        "echo",
			Version:     "1.0.0",
			Description: "Returns the input payload and adds trace metadata.",
			SourceDir:   filepath.Join(projectRoot, "sampleplugins", "echo"),
		},
		{
			Name:        "stats",
			Version:     "1.0.0",
			Description: "Computes simple metrics for the input payload.",
			SourceDir:   filepath.Join(projectRoot, "sampleplugins", "stats"),
		},
		{
			Name:        "audit",
			Version:     "1.0.0",
			Description: "Demonstrates dependency validation against the echo plugin.",
			SourceDir:   filepath.Join(projectRoot, "sampleplugins", "audit"),
		},
		{
			Name:        "unstable",
			Version:     "1.0.0",
			Description: "Always fails and exercises fallback degradation.",
			SourceDir:   filepath.Join(projectRoot, "sampleplugins", "unstable"),
		},
	}

	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("create plugin root: %w", err)
	}

	for _, sample := range samples {
		if err := buildOne(sample, pluginDir); err != nil {
			return err
		}
	}

	return nil
}

func buildOne(sample SamplePlugin, pluginDir string) error {
	targetDir := filepath.Join(pluginDir, sample.Name)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create target dir for %s: %w", sample.Name, err)
	}

	executableName := sample.Name
	if runtime.GOOS == "windows" {
		executableName += ".exe"
	}
	executablePath := filepath.Join(targetDir, executableName)

	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", executablePath, sample.SourceDir)
	cmd.Env = append(os.Environ(), "GOTELEMETRY=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build sample plugin %s: %w\n%s", sample.Name, err, output)
	}

	defaultEnabled := true
	manifest := plugins.Manifest{
		Name:        sample.Name,
		Version:     sample.Version,
		APIVersion:  plugins.HostAPIVersion,
		Description: sample.Description,
		Entry:       executableName,
		Enabled:     &defaultEnabled,
	}
	switch sample.Name {
	case "audit":
		manifest.Dependencies = []plugins.Dependency{{
			Name:       "echo",
			MinVersion: "1.0.0",
		}}
	case "unstable":
		manifest.FailurePolicy = plugins.FailurePolicy{
			UseFallback: true,
			FallbackOutput: map[string]any{
				"mode":    "fallback",
				"message": "unstable plugin degraded gracefully",
			},
		}
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest for %s: %w", sample.Name, err)
	}

	if err := os.WriteFile(filepath.Join(targetDir, "plugin.json"), body, 0o644); err != nil {
		return fmt.Errorf("write manifest for %s: %w", sample.Name, err)
	}

	return nil
}
