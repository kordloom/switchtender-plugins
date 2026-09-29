# switchtender-plugins

Official plugins for [SwitchTender](https://switchtender.com). One binary, `switchtender-notify`,
delivers terminal runs to Discord, ntfy, and Microsoft Teams. Drop it into the server's
`--plugins-dir` and it registers the channels you configure. No recompile of SwitchTender.

This repo is also the reference for writing your own plugin: it builds against
`github.com/kordloom/switchtender/sdk/plugin` exactly the way the
[Extend in Go guide](https://switchtender.com/docs/sdk) describes. Apache-2.0, copy freely.

## Install

Grab a release binary, or build from source:

    go build -o switchtender-notify .

Put it in a directory and point the server (and any workers) at it:

    mkdir -p plugins && mv switchtender-notify plugins/
    SWITCHTENDER_NTFY_URL=https://ntfy.sh/my-long-random-topic \
      switchtender serve --plugins-dir ./plugins

The server log shows `switchtender-notify: serving ntfy` at startup. Every terminal top-level run
now notifies each configured channel.

## Channels

| Channel | Environment | Notes |
|---------|-------------|-------|
| discord | `SWITCHTENDER_DISCORD_WEBHOOK` | A Discord webhook URL. One embed per run, color by status. |
| ntfy | `SWITCHTENDER_NTFY_URL`, optional `SWITCHTENDER_NTFY_TOKEN` | Full topic URL, ntfy.sh or self-hosted. Failed runs publish at high priority. |
| teams | `SWITCHTENDER_TEAMS_WEBHOOK` | A Teams Workflows incoming-webhook URL. Sends an Adaptive Card. |

Set `SWITCHTENDER_NOTIFY_LINK_BASE` to your SwitchTender URL, such as `https://yard.example.com`,
and every notification links straight to the run.

All variables are read by the plugin process, which inherits the server's environment: set them
where the server runs. A channel with no variable set is simply not served; with nothing set the
plugin exits with a clear message in the server log.

## What a notification carries

Run id, a label (the playbook, or the tool when there is no playbook), the tool, the status, the
exit code, the duration, and the run link when a link base is set. It never carries the run's
command body: for bash, python, powershell, and go runs the command is the whole script, and a
script does not belong on an external channel. Extra vars are already redacted by the server
before any notifier sees the run.

## Delivery semantics

SwitchTender delivers to each channel once per terminal top-level run (shards and pipeline steps
do not notify), with a five second timeout. A failed delivery is logged by the server and
dropped. This is best-effort notification, not a guaranteed queue: a burst of runs beyond a
channel's rate limit (Discord allows roughly 30 webhook messages per minute) drops the excess.

On ntfy.sh the topic name is the only secret, so use a long random topic or a self-hosted ntfy
server with authentication and `SWITCHTENDER_NTFY_TOKEN`.

## Future work

Per-template channel routing, delivery retries, message templates, and threads or mentions are
deliberate omissions in this version.
