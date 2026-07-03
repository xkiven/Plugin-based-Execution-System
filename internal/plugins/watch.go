package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type WatchEvent struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
	Plugin  string    `json:"plugin"`
	Details string    `json:"details,omitempty"`
}

func (m *Manager) Watch(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	previous, err := m.Discover()
	if err != nil {
		return err
	}
	if err := emitWatchEvents([]WatchEvent{{
		Time:    time.Now(),
		Type:    "snapshot",
		Plugin:  "*",
		Details: fmt.Sprintf("%d plugins discovered", len(previous)),
	}}); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			current, err := m.Discover()
			if err != nil {
				return err
			}

			events := diffPlugins(previous, current)
			if len(events) > 0 {
				if err := emitWatchEvents(events); err != nil {
					return err
				}
			}
			previous = current
		}
	}
}

func diffPlugins(previous, current []Plugin) []WatchEvent {
	now := time.Now()
	previousIndex := map[string]Plugin{}
	currentIndex := map[string]Plugin{}
	for _, plugin := range previous {
		previousIndex[plugin.Name] = plugin
	}
	for _, plugin := range current {
		currentIndex[plugin.Name] = plugin
	}

	var events []WatchEvent
	for name, plugin := range currentIndex {
		existing, ok := previousIndex[name]
		if !ok {
			events = append(events, WatchEvent{
				Time:    now,
				Type:    "loaded",
				Plugin:  plugin.Name,
				Details: fmt.Sprintf("version=%s status=%s", plugin.Version, plugin.Status),
			})
			continue
		}
		if plugin.Version != existing.Version || plugin.Status != existing.Status || plugin.Enabled != existing.Enabled || plugin.Entry != existing.Entry {
			events = append(events, WatchEvent{
				Time:    now,
				Type:    "updated",
				Plugin:  plugin.Name,
				Details: fmt.Sprintf("version=%s status=%s enabled=%t", plugin.Version, plugin.Status, plugin.Enabled),
			})
		}
	}

	for name := range previousIndex {
		if _, ok := currentIndex[name]; !ok {
			events = append(events, WatchEvent{
				Time:   now,
				Type:   "unloaded",
				Plugin: name,
			})
		}
	}

	sortWatchEvents(events)
	return events
}

func sortWatchEvents(events []WatchEvent) {
	for i := 0; i < len(events); i++ {
		for j := i + 1; j < len(events); j++ {
			if events[j].Plugin < events[i].Plugin || (events[j].Plugin == events[i].Plugin && events[j].Type < events[i].Type) {
				events[i], events[j] = events[j], events[i]
			}
		}
	}
}

func emitWatchEvents(events []WatchEvent) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	return nil
}
