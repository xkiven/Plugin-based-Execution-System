package main

import (
	"context"
	"log"

	"pluginexec/sdk"
)

type plugin struct{}

func (plugin) Name() string {
	return "audit"
}

func (plugin) Version() string {
	return "1.0.0"
}

func (plugin) Run(_ context.Context, data map[string]any) (map[string]any, error) {
	return map[string]any{
		"plugin_count_hint": 1,
		"request_id":        data["request_id"],
		"status":            "audit completed",
	}, nil
}

func main() {
	if err := sdk.Serve(plugin{}); err != nil {
		log.Fatal(err)
	}
}
