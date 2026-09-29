// Package notify delivers terminal SwitchTender runs to external notification channels. Every
// channel formats the same reduced Event, which carries the run's identity and outcome and
// never its command body: a bash or python run's command is the whole script, and a script
// does not belong on an external channel. Extra vars arrive already redacted by the server.
package notify

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kordloom/switchtender/sdk"
)

// labelCap bounds the label so no channel's title limit is ever hit.
const labelCap = 200

// Event is the channel-independent view of a terminal run.
type Event struct {
	// ID is the run identifier.
	ID string
	// Label names the run for a human: the playbook when set, otherwise the tool.
	Label string
	// Tool is the execution tool, with the empty default normalized to ansible.
	Tool string
	// Status is the terminal lifecycle state.
	Status string
	// ExitCode is the process exit code, nil when the run never executed.
	ExitCode *int
	// Duration is how long execution took, zero when the run never started or ended.
	Duration time.Duration
	// Link is the run's UI URL, empty when no link base is configured.
	Link string
}

// FromRun reduces a run to the fields a channel may carry, applying the payload policy.
func FromRun(r *sdk.Run, linkBase string) Event {
	tool := r.Tool
	if tool == "" {
		tool = "ansible"
	}
	label := r.Playbook
	if label == "" {
		label = tool
	}
	if len(label) > labelCap {
		label = label[:labelCap]
	}
	var duration time.Duration
	if r.StartedAt != nil && r.EndedAt != nil {
		duration = r.EndedAt.Sub(*r.StartedAt).Round(time.Millisecond)
	}
	link := ""
	if linkBase != "" {
		link = strings.TrimRight(linkBase, "/") + "/ui/runs/" + r.ID
	}
	return Event{
		ID:       r.ID,
		Label:    label,
		Tool:     tool,
		Status:   string(r.Status),
		ExitCode: r.ExitCode,
		Duration: duration,
		Link:     link,
	}
}

// Title is the one-line summary every channel leads with.
func (e Event) Title() string {
	return e.Label + " " + e.Status
}

// Facts returns the ordered detail pairs a channel renders after the title.
func (e Event) Facts() [][2]string {
	facts := [][2]string{{"Tool", e.Tool}, {"Run", e.ID}}
	if e.ExitCode != nil {
		facts = append(facts, [2]string{"Exit code", strconv.Itoa(*e.ExitCode)})
	}
	if e.Duration > 0 {
		facts = append(facts, [2]string{"Duration", e.Duration.String()})
	}
	return facts
}

// post sends one payload and reports a non-success reply as ErrDeliver. The caller's context
// carries the server's delivery timeout.
func post(ctx context.Context, client *http.Client, url, contentType string, body []byte, header http.Header) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDeliver, err)
	}
	req.Header.Set("Content-Type", contentType)
	for key, values := range header {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDeliver, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status %d", ErrDeliver, resp.StatusCode)
	}
	return nil
}
