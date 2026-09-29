package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kordloom/switchtender/sdk"
)

// teamsColors maps a run status to an Adaptive Card text color.
var teamsColors = map[string]string{
	"succeeded":   "Good",
	"failed":      "Attention",
	"canceled":    "Default",
	"interrupted": "Warning",
	"rejected":    "Warning",
}

// teamsMessage is the payload a Teams Workflows webhook accepts: an Adaptive Card attachment.
// The legacy Office 365 connector card is not used; Microsoft retired connectors in 2025.
type teamsMessage struct {
	// Type is always message.
	Type string `json:"type"`
	// Attachments carries the single Adaptive Card.
	Attachments []teamsAttachment `json:"attachments"`
}

// teamsAttachment wraps the Adaptive Card content.
type teamsAttachment struct {
	// ContentType marks the attachment as an Adaptive Card.
	ContentType string `json:"contentType"`
	// Content is the card itself.
	Content teamsCard `json:"content"`
}

// teamsCard is the Adaptive Card body.
type teamsCard struct {
	// Schema is the Adaptive Card schema URL.
	Schema string `json:"$schema"`
	// Type is always AdaptiveCard.
	Type string `json:"type"`
	// Version is the card schema version.
	Version string `json:"version"`
	// Body holds the title block and the fact set.
	Body []map[string]any `json:"body"`
	// Actions holds the open-run action when a link base is configured.
	Actions []map[string]any `json:"actions,omitempty"`
}

// Teams returns a notifier that posts each terminal run to a Microsoft Teams Workflows
// incoming-webhook URL as an Adaptive Card, colored by status.
func Teams(webhook, linkBase string, client *http.Client) sdk.Notifier {
	return sdk.NotifierFunc(func(ctx context.Context, r *sdk.Run) error {
		event := FromRun(r, linkBase)
		color, ok := teamsColors[event.Status]
		if !ok {
			color = "Default"
		}
		facts := make([]map[string]string, 0, 4)
		for _, fact := range event.Facts() {
			facts = append(facts, map[string]string{"title": fact[0], "value": fact[1]})
		}
		card := teamsCard{
			Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
			Type:    "AdaptiveCard",
			Version: "1.4",
			Body: []map[string]any{
				{"type": "TextBlock", "text": event.Title(), "weight": "Bolder", "size": "Medium", "color": color, "wrap": true},
				{"type": "FactSet", "facts": facts},
			},
		}
		if event.Link != "" {
			card.Actions = []map[string]any{
				{"type": "Action.OpenUrl", "title": "Open the run", "url": event.Link},
			}
		}
		body, err := json.Marshal(teamsMessage{
			Type: "message",
			Attachments: []teamsAttachment{
				{ContentType: "application/vnd.microsoft.card.adaptive", Content: card},
			},
		})
		if err != nil {
			return fmt.Errorf("teams: %w", err)
		}
		if err := post(ctx, client, webhook, "application/json", body, nil); err != nil {
			return fmt.Errorf("teams: %w", err)
		}
		return nil
	})
}
