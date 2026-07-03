package main

import (
	"context"
	"log"

	"pluginexec/sdk"
)

type plugin struct{}

func (plugin) Name() string {
	return "echo"
}

func (plugin) Version() string {
	return "1.0.0"
}

func (plugin) Run(_ context.Context, data map[string]any) (map[string]any, error) {
	return map[string]any{
		"received": data,
		"message":  "echo plugin completed",
	}, nil
}

func main() {
	if err := sdk.Serve(plugin{}); err != nil {
		log.Fatal(err)
	}
}
