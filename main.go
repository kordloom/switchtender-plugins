// Command switchtender-notify serves Discord, ntfy, and Microsoft Teams notification channels as
// one SwitchTender plugin. Drop the binary into the server's --plugins-dir and set an environment
// variable per channel on the server process; the plugin registers only the channels that are
// configured. SwitchTender delivers every terminal top-level run with extra vars already redacted,
// and this plugin additionally never forwards a run's command body.
package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kordloom/switchtender/sdk"
	"github.com/kordloom/switchtender/sdk/plugin"

	"github.com/kordloom/switchtender-plugins/notify"
)

// deliverTimeout bounds one delivery attempt, matching the server's own notification timeout.
const deliverTimeout = 5 * time.Second

// buildExtension assembles the served channels from the environment. It returns a nil extension
// when no channel is configured, and the list of configured channel names for the startup log.
func buildExtension(getenv func(string) string) (*plugin.Extension, string) {
	client := &http.Client{Timeout: deliverTimeout}
	linkBase := getenv("SWITCHTENDER_PLUGIN_NOTIFY_LINK_BASE")
	notifiers := map[string]sdk.Notifier{}
	var channels []string
	if webhook := getenv("SWITCHTENDER_PLUGIN_DISCORD_WEBHOOK"); webhook != "" {
		notifiers["discord"] = notify.Discord(webhook, linkBase, client)
		channels = append(channels, "discord")
	}
	if topicURL := getenv("SWITCHTENDER_PLUGIN_NTFY_URL"); topicURL != "" {
		notifiers["ntfy"] = notify.Ntfy(topicURL, getenv("SWITCHTENDER_PLUGIN_NTFY_TOKEN"), linkBase,
			client)
		channels = append(channels, "ntfy")
	}
	if webhook := getenv("SWITCHTENDER_PLUGIN_TEAMS_WEBHOOK"); webhook != "" {
		notifiers["teams"] = notify.Teams(webhook, linkBase, client)
		channels = append(channels, "teams")
	}
	if instance := getenv("SWITCHTENDER_PLUGIN_SERVICENOW_URL"); instance != "" {
		notifiers["servicenow"] = notify.ServiceNow(instance,
			getenv("SWITCHTENDER_PLUGIN_SERVICENOW_USER"), getenv("SWITCHTENDER_PLUGIN_SERVICENOW_PASSWORD"),
			getenv("SWITCHTENDER_PLUGIN_SERVICENOW_TOKEN"), linkBase, client)
		channels = append(channels, "servicenow")
	}
	if base := getenv("SWITCHTENDER_PLUGIN_JIRA_URL"); base != "" {
		notifiers["jira"] = notify.Jira(base, getenv("SWITCHTENDER_PLUGIN_JIRA_USER"),
			getenv("SWITCHTENDER_PLUGIN_JIRA_TOKEN"), linkBase, client)
		channels = append(channels, "jira")
	}
	if len(notifiers) == 0 {
		return nil, ""
	}
	return &plugin.Extension{Notifiers: notifiers}, strings.Join(channels, ", ")
}

// main serves the configured channels, or exits loudly when none are configured so the server
// log shows exactly why the plugin refused to start.
func main() {
	ext, channels := buildExtension(os.Getenv)
	if ext == nil {
		fmt.Fprintln(os.Stderr, "switchtender-notify: no channel configured. Set "+
			"SWITCHTENDER_PLUGIN_DISCORD_WEBHOOK, SWITCHTENDER_PLUGIN_NTFY_URL, "+
			"SWITCHTENDER_PLUGIN_TEAMS_WEBHOOK, SWITCHTENDER_PLUGIN_SERVICENOW_URL, or "+
			"SWITCHTENDER_PLUGIN_JIRA_URL.")
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "switchtender-notify: serving "+channels)
	plugin.Serve(ext)
}
