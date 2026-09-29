package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kordloom/switchtender/sdk"
)

// discordColors maps a run status to a Discord embed color.
var discordColors = map[string]int{
	"succeeded":   0x2ECC71,
	"failed":      0xE74C3C,
	"canceled":    0x95A5A6,
	"interrupted": 0xE67E22,
	"rejected":    0xE67E22,
}

// discordMessage is the webhook payload: one embed per notification.
type discordMessage struct {
	// Embeds carries the single run embed.
	Embeds []discordEmbed `json:"embeds"`
}

// discordEmbed is one Discord embed.
type discordEmbed struct {
	// Title is the run label and status.
	Title string `json:"title"`
	// URL makes the title link to the run when a link base is configured.
	URL string `json:"url,omitempty"`
	// Color is the embed accent color, chosen by status.
	Color int `json:"color"`
	// Fields carries the run facts.
	Fields []discordField `json:"fields,omitempty"`
}

// discordField is one embed fact.
type discordField struct {
	// Name is the fact label.
	Name string `json:"name"`
	// Value is the fact value.
	Value string `json:"value"`
	// Inline packs facts side by side.
	Inline bool `json:"inline"`
}

// Discord returns a notifier that posts each terminal run to a Discord webhook as one embed,
// colored by status.
func Discord(webhook, linkBase string, client *http.Client) sdk.Notifier {
	return sdk.NotifierFunc(func(ctx context.Context, r *sdk.Run) error {
		event := FromRun(r, linkBase)
		embed := discordEmbed{
			Title: event.Title(),
			URL:   event.Link,
			Color: discordColors[event.Status],
		}
		for _, fact := range event.Facts() {
			embed.Fields = append(embed.Fields, discordField{Name: fact[0], Value: fact[1], Inline: true})
		}
		body, err := json.Marshal(discordMessage{Embeds: []discordEmbed{embed}})
		if err != nil {
			return fmt.Errorf("discord: %w", err)
		}
		if err := post(ctx, client, webhook, "application/json", body, nil); err != nil {
			return fmt.Errorf("discord: %w", err)
		}
		return nil
	})
}
