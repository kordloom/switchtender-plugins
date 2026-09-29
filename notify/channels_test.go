package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kordloom/switchtender/sdk"
)

// capture records the one request a channel test delivers.
type capture struct {
	// Header is the delivered request header.
	Header http.Header
	// Body is the delivered request body.
	Body []byte
}

// captureServer returns a test server recording each request into out and replying with status.
func captureServer(t *testing.T, out *capture, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		out.Header = r.Header.Clone()
		out.Body = body
		w.WriteHeader(status)
	}))
}

// terminalRun builds the run every channel test delivers: a failed bash run whose command body
// must never appear in a payload.
func terminalRun() *sdk.Run {
	start := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Second)
	exit := 7
	return &sdk.Run{
		ID: "run_ch", Tool: "bash", Command: "echo topsecretscript", Status: "failed",
		ExitCode: &exit, StartedAt: &start, EndedAt: &end,
	}
}

func TestDiscordDelivers(t *testing.T) {
	t.Parallel()
	var got capture
	srv := captureServer(t, &got, http.StatusNoContent)
	defer srv.Close()

	err := Discord(srv.URL, "https://yard.example.com", srv.Client()).
		Notify(context.Background(), terminalRun())
	if err != nil {
		t.Fatalf("Notify error: %v", err)
	}
	var msg struct {
		Embeds []struct {
			Title  string `json:"title"`
			URL    string `json:"url"`
			Color  int    `json:"color"`
			Fields []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"fields"`
		} `json:"embeds"`
	}
	if err := json.Unmarshal(got.Body, &msg); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(msg.Embeds) != 1 {
		t.Fatalf("embeds = %d, want 1", len(msg.Embeds))
	}
	embed := msg.Embeds[0]
	if embed.Title != "bash failed" {
		t.Errorf("title = %q, want bash failed", embed.Title)
	}
	if embed.Color != discordColors["failed"] {
		t.Errorf("color = %d, want the failed color", embed.Color)
	}
	if embed.URL != "https://yard.example.com/ui/runs/run_ch" {
		t.Errorf("url = %q, want the run link", embed.URL)
	}
	if len(embed.Fields) != 4 {
		t.Errorf("fields = %d, want 4", len(embed.Fields))
	}
	if strings.Contains(string(got.Body), "topsecretscript") {
		t.Error("payload carries the command body, want it withheld")
	}
}

func TestNtfyDelivers(t *testing.T) {
	t.Parallel()
	var got capture
	srv := captureServer(t, &got, http.StatusOK)
	defer srv.Close()

	err := Ntfy(srv.URL, "tok123", "https://yard.example.com", srv.Client()).
		Notify(context.Background(), terminalRun())
	if err != nil {
		t.Fatalf("Notify error: %v", err)
	}
	if got.Header.Get("Title") != "bash failed" {
		t.Errorf("Title header = %q, want bash failed", got.Header.Get("Title"))
	}
	if got.Header.Get("Priority") != "high" {
		t.Errorf("Priority = %q, want high for a failed run", got.Header.Get("Priority"))
	}
	if got.Header.Get("Tags") != "failed" {
		t.Errorf("Tags = %q, want failed", got.Header.Get("Tags"))
	}
	if got.Header.Get("Authorization") != "Bearer tok123" {
		t.Errorf("Authorization = %q, want the bearer token", got.Header.Get("Authorization"))
	}
	if got.Header.Get("X-Click") != "https://yard.example.com/ui/runs/run_ch" {
		t.Errorf("X-Click = %q, want the run link", got.Header.Get("X-Click"))
	}
	body := string(got.Body)
	if !strings.Contains(body, "Exit code: 7") || !strings.Contains(body, "Tool: bash") {
		t.Errorf("body = %q, want the run facts", body)
	}
	if strings.Contains(body, "topsecretscript") {
		t.Error("payload carries the command body, want it withheld")
	}
}

func TestNtfySucceededDefaultPriority(t *testing.T) {
	t.Parallel()
	var got capture
	srv := captureServer(t, &got, http.StatusOK)
	defer srv.Close()

	run := terminalRun()
	run.Status = "succeeded"
	if err := Ntfy(srv.URL, "", "", srv.Client()).Notify(context.Background(), run); err != nil {
		t.Fatalf("Notify error: %v", err)
	}
	if got.Header.Get("Priority") != "default" {
		t.Errorf("Priority = %q, want default for a succeeded run", got.Header.Get("Priority"))
	}
	if got.Header.Get("Authorization") != "" {
		t.Errorf("Authorization = %q, want none without a token", got.Header.Get("Authorization"))
	}
}

func TestTeamsDelivers(t *testing.T) {
	t.Parallel()
	var got capture
	srv := captureServer(t, &got, http.StatusAccepted)
	defer srv.Close()

	err := Teams(srv.URL, "https://yard.example.com", srv.Client()).
		Notify(context.Background(), terminalRun())
	if err != nil {
		t.Fatalf("Notify error: %v", err)
	}
	body := string(got.Body)
	if !strings.Contains(body, `"application/vnd.microsoft.card.adaptive"`) {
		t.Error("payload is not an Adaptive Card attachment")
	}
	if !strings.Contains(body, `"bash failed"`) {
		t.Error("payload misses the title")
	}
	if !strings.Contains(body, `"Attention"`) {
		t.Error("payload misses the failed color")
	}
	if !strings.Contains(body, "Action.OpenUrl") {
		t.Error("payload misses the open-run action")
	}
	if strings.Contains(body, "topsecretscript") {
		t.Error("payload carries the command body, want it withheld")
	}
}

func TestDeliveryFailureSurfaces(t *testing.T) {
	t.Parallel()
	var got capture
	srv := captureServer(t, &got, http.StatusTooManyRequests)
	defer srv.Close()

	err := Discord(srv.URL, "", srv.Client()).Notify(context.Background(), terminalRun())
	if !errors.Is(err, ErrDeliver) {
		t.Errorf("error = %v, want ErrDeliver on a non-2xx reply", err)
	}
}
