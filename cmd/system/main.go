package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pluginexec/internal/plugins"
	"pluginexec/internal/samplebuild"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	pluginDir := filepath.Join(projectRoot, "plugins")
	manager := plugins.NewManager(pluginDir)

	switch args[0] {
	case "build-samples":
		return samplebuild.BuildSamples(projectRoot, pluginDir)
	case "list":
		found, err := manager.Discover()
		if err != nil {
			return err
		}
		return printJSON(found)
	case "enable":
		if len(args) < 2 {
			return errors.New("enable requires a plugin name")
		}
		return manager.Enable(args[1])
	case "disable":
		if len(args) < 2 {
			return errors.New("disable requires a plugin name")
		}
		return manager.Disable(args[1])
	case "run":
		runFlags := flag.NewFlagSet("run", flag.ContinueOnError)
		inputPath := runFlags.String("input", "", "path to input json")
		timeout := runFlags.Duration("timeout", 3*time.Second, "per-plugin timeout")
		if err := runFlags.Parse(args[1:]); err != nil {
			return err
		}
		if *inputPath == "" {
			return errors.New("run requires -input")
		}

		payload, err := readInput(*inputPath)
		if err != nil {
			return err
		}

		summary, err := manager.ExecuteAll(context.Background(), payload, *timeout)
		if err != nil {
			return err
		}
		return printJSON(summary)
	case "watch":
		watchFlags := flag.NewFlagSet("watch", flag.ContinueOnError)
		interval := watchFlags.Duration("interval", 2*time.Second, "plugin directory rescan interval")
		if err := watchFlags.Parse(args[1:]); err != nil {
			return err
		}
		return manager.Watch(context.Background(), *interval)
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func readInput(path string) (map[string]any, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read input file: %w", err)
	}

	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("decode input file: %w", err)
	}
	return data, nil
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printUsage() {
	fmt.Println(`Plugin-based Execution System

Commands:
  build-samples        Build sample plugins into ./plugins
  list                 List plugins and their current status
  enable <name>        Enable a plugin
  disable <name>       Disable a plugin
  watch                Watch plugin directory and emit load/unload/update events
  run -input <file>    Execute all enabled plugins with the input JSON`)
}
