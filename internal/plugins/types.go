package plugins

import "time"

const HostAPIVersion = "1.0.0"

type Status string

const (
	StatusEnabled  Status = "enabled"
	StatusDisabled Status = "disabled"
	StatusError    Status = "error"
)

type Manifest struct {
	Name                   string        `json:"name"`
	Version                string        `json:"version"`
	APIVersion             string        `json:"api_version,omitempty"`
	Entry                  string        `json:"entry"`
	Description            string        `json:"description,omitempty"`
	Enabled                *bool         `json:"enabled,omitempty"`
	Runtime                RuntimeSpec   `json:"runtime,omitempty"`
	Dependencies           []Dependency  `json:"dependencies,omitempty"`
	FailurePolicy          FailurePolicy `json:"failure_policy,omitempty"`
	CompatibleHostVersions []string      `json:"compatible_host_versions,omitempty"`
}

type State struct {
	Overrides map[string]bool    `json:"overrides,omitempty"`
	LastRuns  map[string]RunInfo `json:"last_runs,omitempty"`
}

type RunInfo struct {
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Plugin struct {
	Name                   string        `json:"name"`
	Version                string        `json:"version"`
	APIVersion             string        `json:"api_version,omitempty"`
	Description            string        `json:"description,omitempty"`
	Entry                  string        `json:"entry"`
	Directory              string        `json:"directory"`
	Enabled                bool          `json:"enabled"`
	Status                 Status        `json:"status"`
	LastError              string        `json:"last_error,omitempty"`
	UpdatedAt              time.Time     `json:"updated_at,omitempty"`
	Runtime                RuntimeSpec   `json:"runtime,omitempty"`
	Dependencies           []Dependency  `json:"dependencies,omitempty"`
	FailurePolicy          FailurePolicy `json:"failure_policy,omitempty"`
	CompatibleHostVersions []string      `json:"compatible_host_versions,omitempty"`
}

type ExecutionResult struct {
	Plugin      string         `json:"plugin"`
	Version     string         `json:"version"`
	Status      string         `json:"status"`
	Duration    time.Duration  `json:"duration"`
	Output      map[string]any `json:"output,omitempty"`
	Error       string         `json:"error,omitempty"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt time.Time      `json:"completed_at"`
}

type ExecutionSummary struct {
	StartedAt   time.Time         `json:"started_at"`
	CompletedAt time.Time         `json:"completed_at"`
	Input       map[string]any    `json:"input"`
	Results     []ExecutionResult `json:"results"`
}

type RuntimeSpec struct {
	Type    string   `json:"type,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

type Dependency struct {
	Name       string `json:"name"`
	MinVersion string `json:"min_version,omitempty"`
}

type FailurePolicy struct {
	UseFallback    bool           `json:"use_fallback,omitempty"`
	FallbackOutput map[string]any `json:"fallback_output,omitempty"`
}
