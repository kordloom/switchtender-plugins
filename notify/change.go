package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/kordloom/switchtender/sdk"
)

// changeLabel is the run label SwitchTender already gathers runs into one change by. A team that
// labels its runs with a change ticket's number gets each run recorded on that ticket.
const changeLabel = "change"

// ticketReplyCap bounds how much of a ticketing system's reply is read.
const ticketReplyCap = 1 << 20

// changeNote is the text a change ticket receives for one run: what ran and how it ended, the
// command that exports its evidence, and, when a link base is set, links to the run and to every
// run on the same change. Like every channel it carries the run's identity and outcome and never
// its command body.
func changeNote(r *sdk.Run, change, linkBase string) string {
	event := FromRun(r, linkBase)
	var b strings.Builder
	fmt.Fprintf(&b, "SwitchTender run %s %s: %s", event.ID, event.Status, event.Label)
	if event.Tool != event.Label {
		fmt.Fprintf(&b, " (%s)", event.Tool)
	}
	if event.ExitCode != nil {
		fmt.Fprintf(&b, ", exit code %d", *event.ExitCode)
	}
	if event.Duration > 0 {
		fmt.Fprintf(&b, ", %s", event.Duration)
	}
	b.WriteString(".")
	if event.Link != "" {
		fmt.Fprintf(&b, "\nRun: %s", event.Link)
		fmt.Fprintf(&b, "\nEvery run on this change: %s/ui/runs?q=%s", strings.TrimRight(linkBase, "/"),
			url.QueryEscape("label:"+changeLabel+"="+change))
	}
	fmt.Fprintf(&b, "\nEvidence file: switchtender audit run %s", event.ID)
	return b.String()
}

// ticketAuth returns what sets a ticketing request's credentials: basic auth when a user is
// given, and the secret as a bearer token when it is not.
func ticketAuth(user, secret string) func(*http.Request) {
	return func(req *http.Request) {
		if user == "" {
			req.Header.Set("Authorization", "Bearer "+secret)
			return
		}
		req.SetBasicAuth(user, secret)
	}
}

// ticketRequest sends one JSON request to a ticketing system and returns its reply, reporting a
// failed or non-success exchange as ErrDeliver. A nil body sends none.
func ticketRequest(ctx context.Context, client *http.Client, method, target string, body any,
	auth func(*http.Request)) ([]byte, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrDeliver, err)
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDeliver, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	auth(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDeliver, err)
	}
	defer func() { _ = resp.Body.Close() }()
	reply, err := io.ReadAll(io.LimitReader(resp.Body, ticketReplyCap))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDeliver, err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d", ErrDeliver, resp.StatusCode)
	}
	return reply, nil
}
