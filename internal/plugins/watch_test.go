package plugins

import "testing"

func TestDiffPluginsReportsLoadedUpdatedAndUnloaded(t *testing.T) {
	previous := []Plugin{
		{
			Name:    "beta",
			Version: "1.0.0",
			Enabled: true,
			Status:  StatusEnabled,
			Entry:   "beta-v1",
		},
		{
			Name:    "alpha",
			Version: "1.0.0",
			Enabled: true,
			Status:  StatusEnabled,
			Entry:   "alpha-v1",
		},
	}
	current := []Plugin{
		{
			Name:    "alpha",
			Version: "2.0.0",
			Enabled: true,
			Status:  StatusEnabled,
			Entry:   "alpha-v2",
		},
		{
			Name:    "gamma",
			Version: "1.0.0",
			Enabled: true,
			Status:  StatusEnabled,
			Entry:   "gamma-v1",
		},
	}

	events := diffPlugins(previous, current)
	if len(events) != 3 {
		t.Fatalf("unexpected event count: %#v", events)
	}

	if events[0].Plugin != "alpha" || events[0].Type != "updated" {
		t.Fatalf("unexpected first event: %#v", events[0])
	}
	if events[1].Plugin != "beta" || events[1].Type != "unloaded" {
		t.Fatalf("unexpected second event: %#v", events[1])
	}
	if events[2].Plugin != "gamma" || events[2].Type != "loaded" {
		t.Fatalf("unexpected third event: %#v", events[2])
	}
}
