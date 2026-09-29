package main

import (
	"fmt"
	"testing"
)

// TestBuildExtension confirms the extension serves exactly the channels the environment
// configures, and refuses to serve when nothing is configured.
func TestBuildExtension(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Env          map[string]string
		WantChannels string
		WantNil      bool
	}{{ // Test 0: Nothing configured builds no extension.
		Env: map[string]string{}, WantNil: true,
	}, { // Test 1: One channel serves alone.
		Env:          map[string]string{"SWITCHTENDER_NTFY_URL": "https://ntfy.sh/t"},
		WantChannels: "ntfy",
	}, { // Test 2: All three channels serve together.
		Env: map[string]string{
			"SWITCHTENDER_DISCORD_WEBHOOK": "https://discord.example/hook",
			"SWITCHTENDER_NTFY_URL":        "https://ntfy.sh/t",
			"SWITCHTENDER_TEAMS_WEBHOOK":   "https://teams.example/hook",
		},
		WantChannels: "discord, ntfy, teams",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			ext, channels := buildExtension(func(key string) string { return test.Env[key] })
			if test.WantNil {
				if ext != nil {
					t.Fatalf("extension = %v, want nil with nothing configured", ext)
				}
				return
			}
			if ext == nil {
				t.Fatal("extension = nil, want channels served")
			}
			if channels != test.WantChannels {
				t.Errorf("channels = %q, want %q", channels, test.WantChannels)
			}
			if len(ext.Notifiers) != len(test.Env) {
				t.Errorf("notifiers = %d, want one per configured channel", len(ext.Notifiers))
			}
		})
	}
}
