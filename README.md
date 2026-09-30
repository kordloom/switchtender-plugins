# switchtender-plugins

Official plugins for [SwitchTender](https://switchtender.com). One binary, `switchtender-notify`,
delivers terminal runs to Discord, ntfy, and Microsoft Teams, and records them on ServiceNow change
requests and Jira issues. Drop it into the server's `--plugins-dir` and it registers the channels
you configure. No recompile of SwitchTender.

This repo is also the reference for writing your own plugin: it builds against
`github.com/kordloom/switchtender/sdk/plugin` exactly the way the
[Extend in Go guide](https://switchtender.com/docs/sdk) describes. Apache-2.0, copy freely.

## Install

Grab a release binary, or build from source:

    go build -o switchtender-notify .

Put it in a directory and point the server (and any workers) at it:

    mkdir -p plugins && mv switchtender-notify plugins/
    SWITCHTENDER_PLUGIN_NTFY_URL=https://ntfy.sh/my-long-random-topic \
      switchtender serve --plugins-dir ./plugins

The server log shows `switchtender-notify: serving ntfy` at startup. Every terminal top-level run
now notifies each configured channel.

## Channels

| Channel | Environment | Notes |
|---------|-------------|-------|
| discord | `SWITCHTENDER_PLUGIN_DISCORD_WEBHOOK` | A Discord webhook URL. One embed per run, color by status. |
| ntfy | `SWITCHTENDER_PLUGIN_NTFY_URL`, optional `SWITCHTENDER_PLUGIN_NTFY_TOKEN` | Full topic URL, ntfy.sh or self-hosted. Failed runs publish at high priority. |
| teams | `SWITCHTENDER_PLUGIN_TEAMS_WEBHOOK` | A Teams Workflows incoming-webhook URL. Sends an Adaptive Card. |
| servicenow | `SWITCHTENDER_PLUGIN_SERVICENOW_URL`, plus `SWITCHTENDER_PLUGIN_SERVICENOW_USER` and `SWITCHTENDER_PLUGIN_SERVICENOW_PASSWORD`, or `SWITCHTENDER_PLUGIN_SERVICENOW_TOKEN` | Records each run as a work note on the change request its `change` label names. See Change tickets. |
| jira | `SWITCHTENDER_PLUGIN_JIRA_URL` and `SWITCHTENDER_PLUGIN_JIRA_TOKEN`, plus `SWITCHTENDER_PLUGIN_JIRA_USER` on Jira Cloud | Records each run as a comment on the issue its `change` label names. See Change tickets. |

Set `SWITCHTENDER_PLUGIN_NOTIFY_LINK_BASE` to your SwitchTender URL, such as `https://yard.example.com`,
and every notification links straight to the run.

Set these where the server runs. The server hands a plugin only the variables named with the
`SWITCHTENDER_PLUGIN_` prefix, plus the few any process needs, such as `PATH` and `HOME`, so a
plugin never receives the encryption key or any other secret the server reads from its own
environment. A variable without the prefix never reaches the plugin. A channel with no variable
set is not served, and with nothing set the plugin exits and the server log says it skipped it.

## What a notification carries

Run id, a label (the playbook, or the tool when there is no playbook), the tool, the status, the
exit code, the duration, and the run link when a link base is set. It never carries the run's
command body: for bash, python, powershell, and go runs the command is the whole script, and a
script does not belong on an external channel. Extra vars are already redacted by the server
before any notifier sees the run.

## Change tickets

SwitchTender already gathers runs into one change by their `change` label. Label a run with its
ServiceNow change number, such as `CHG0030001`, or its Jira issue key, such as `OPS-123`, and when
the run ends the servicenow or jira channel records it on that ticket: what ran and how it ended,
the command that exports its evidence file, and, with a link base set, links to the run and to
every run on the change. Set the label when a run is launched through the API, from a job
template, or by an agent's request. A run without a change label, or whose label is neither a
change number nor an issue key, is left alone.

The ServiceNow account needs to read `change_request` records and write their work notes. A token
authenticates as a bearer, otherwise the user and password as basic auth. On Jira Cloud, set the
account's email as the user and an API token as the token. On Jira Data Center, set only a
personal access token. The account needs permission to comment on the issue.

Only an exact change number (`CHG` and digits) or issue key is acted on. A ServiceNow change number
goes into an encoded query, where a caret would add a condition and widen the lookup to another
change, and an issue key goes into a request path, so anything else in the label is ignored rather
than sent.

## Delivery semantics

SwitchTender delivers to each channel once per terminal top-level run (shards and pipeline steps
do not notify), with a five second timeout. A failed delivery is logged by the server and
dropped. This is best-effort notification, not a guaranteed queue: a burst of runs beyond a
channel's rate limit (Discord allows roughly 30 webhook messages per minute) drops the excess.

A ticket that misses a note still has every run in SwitchTender's own change view, which the note
only points at.

On ntfy.sh the topic name is the only secret, so use a long random topic or a self-hosted ntfy
server with authentication and `SWITCHTENDER_PLUGIN_NTFY_TOKEN`.

## Future work

Per-template channel routing, delivery retries, message templates, and threads or mentions are
deliberate omissions in this version.

## History

The commits before v0.5.0 each hold the code of an earlier release, v0.2.0 through v0.4.0, tagged
`snapshot/vX.Y.Z`. Those releases were published from an earlier repository that is no longer
public. Every release from v0.5.0 on comes from this repository.
