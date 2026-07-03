package main

import (
	"context"
	"errors"
	"log"

	"pluginexec/sdk"
)

type plugin struct{}

func (plugin) Name() string {
	return "unstable"
}

func (plugin) Version() string {
	return "1.0.0"
}

func (plugin) Run(_ context.Context, _ map[string]any) (map[string]any, error) {
	return nil, errors.New("simulated plugin failure")
}

func main() {
	if err := sdk.Serve(plugin{}); err != nil {
		log.Fatal(err)
	}
}
