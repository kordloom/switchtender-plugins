package main

import (
	"fmt"
	"strings"
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
		Env:          map[string]string{"SWITCHTENDER_PLUGIN_NTFY_URL": "https://ntfy.sh/t"},
		WantChannels: "ntfy",
	}, { // Test 2: All three channels serve together.
		Env: map[string]string{
			"SWITCHTENDER_PLUGIN_DISCORD_WEBHOOK": "https://discord.example/hook",
			"SWITCHTENDER_PLUGIN_NTFY_URL":        "https://ntfy.sh/t",
			"SWITCHTENDER_PLUGIN_TEAMS_WEBHOOK":   "https://teams.example/hook",
		},
		WantChannels: "discord, ntfy, teams",
	}, { // Test 3: The change ticket channels serve beside the chat channels.
		Env: map[string]string{
			"SWITCHTENDER_PLUGIN_DISCORD_WEBHOOK": "https://discord.example/hook",
			"SWITCHTENDER_PLUGIN_SERVICENOW_URL":  "https://acme.service-now.com",
			"SWITCHTENDER_PLUGIN_JIRA_URL":        "https://acme.atlassian.net",
		},
		WantChannels: "discord, servicenow, jira",
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

// TestEveryVariableCarriesThePluginPrefix pins the contract with the host: the server passes a
// plugin only variables named with the SWITCHTENDER_PLUGIN_ prefix, so a channel reading any other
// name can never be configured. Every channel read one for a release after that rule landed, and
// the plugin quietly served nothing, because the tests above hand buildExtension any name they
// like.
func TestEveryVariableCarriesThePluginPrefix(t *testing.T) {
	t.Parallel()
	var read []string
	buildExtension(func(key string) string {
		read = append(read, key)
		return "https://configured.example"
	})
	if len(read) == 0 {
		t.Fatal("buildExtension read no variables, so this check covers nothing")
	}
	for _, key := range read {
		if !strings.HasPrefix(key, "SWITCHTENDER_PLUGIN_") {
			t.Errorf("buildExtension reads %s, which the server never passes to a plugin", key)
		}
	}
}
