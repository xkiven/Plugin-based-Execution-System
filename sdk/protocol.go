package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Plugin is the contract every executable plugin must implement.
type Plugin interface {
	Name() string
	Version() string
	Run(ctx context.Context, data map[string]any) (map[string]any, error)
}

// ExecuteRequest is sent from host to plugin through stdin.
type ExecuteRequest struct {
	Data map[string]any `json:"data"`
}

// ExecuteResponse is sent from plugin to host through stdout.
type ExecuteResponse struct {
	Output   map[string]any `json:"output,omitempty"`
	Error    string         `json:"error,omitempty"`
	Metadata Metadata       `json:"metadata"`
}

// Metadata is shared by host and plugin for basic identification.
type Metadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Serve reads a request from stdin, invokes the plugin, and writes the response to stdout.
func Serve(plugin Plugin) error {
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	var req ExecuteRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return writeResponse(ExecuteResponse{
				Error: fmt.Sprintf("invalid request: %v", err),
				Metadata: Metadata{
					Name:    plugin.Name(),
					Version: plugin.Version(),
				},
			})
		}
	}

	output, err := plugin.Run(context.Background(), req.Data)
	resp := ExecuteResponse{
		Output: output,
		Metadata: Metadata{
			Name:    plugin.Name(),
			Version: plugin.Version(),
		},
	}
	if err != nil {
		resp.Error = err.Error()
	}

	return writeResponse(resp)
}

func writeResponse(resp ExecuteResponse) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resp)
}
