package notify

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/kordloom/switchtender/sdk"
)

// Ntfy returns a notifier that publishes each terminal run to an ntfy topic URL, such as
// https://ntfy.sh/my-topic or a self-hosted server. A failed run publishes at high priority.
// token, when set, is sent as a bearer token for an authenticated server.
func Ntfy(topicURL, token, linkBase string, client *http.Client) sdk.Notifier {
	return sdk.NotifierFunc(func(ctx context.Context, r *sdk.Run) error {
		event := FromRun(r, linkBase)
		var lines []string
		for _, fact := range event.Facts() {
			lines = append(lines, fact[0]+": "+fact[1])
		}
		header := http.Header{}
		header.Set("Title", event.Title())
		header.Set("Tags", event.Status)
		priority := "default"
		if event.Status == "failed" {
			priority = "high"
		}
		header.Set("Priority", priority)
		if event.Link != "" {
			header.Set("X-Click", event.Link)
		}
		if token != "" {
			header.Set("Authorization", "Bearer "+token)
		}
		body := []byte(strings.Join(lines, "\n"))
		if err := post(ctx, client, topicURL, "text/plain", body, header); err != nil {
			return fmt.Errorf("ntfy: %w", err)
		}
		return nil
	})
}
