package main

import (
	"context"
	"fmt"
	"log"

	"pluginexec/sdk"
)

type plugin struct{}

func (plugin) Name() string {
	return "stats"
}

func (plugin) Version() string {
	return "1.0.0"
}

func (plugin) Run(_ context.Context, data map[string]any) (map[string]any, error) {
	var numberCount int
	var sum float64
	if rawNumbers, ok := data["numbers"]; ok {
		numbers, ok := rawNumbers.([]any)
		if !ok {
			return nil, fmt.Errorf("numbers must be an array")
		}
		for _, item := range numbers {
			value, ok := item.(float64)
			if !ok {
				return nil, fmt.Errorf("numbers must contain only numeric values")
			}
			numberCount++
			sum += value
		}
	}

	return map[string]any{
		"input_key_count": len(data),
		"number_count":    numberCount,
		"number_sum":      sum,
	}, nil
}

func main() {
	if err := sdk.Serve(plugin{}); err != nil {
		log.Fatal(err)
	}
}
