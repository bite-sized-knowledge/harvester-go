package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type DiscordNotifier struct {
	webhookURL string
	client     *http.Client
}

func NewDiscordNotifier(webhookURL string) *DiscordNotifier {
	return &DiscordNotifier{
		webhookURL: strings.TrimSpace(webhookURL),
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (d *DiscordNotifier) Notify(ctx context.Context, articleTitle, articleURL, blogTitle string) error {
	if d.webhookURL == "" {
		return nil
	}

	payload := map[string]any{
		"embeds": []map[string]string{{
			"title":       "새 글이 수집되었어요! ✨",
			"description": fmt.Sprintf("- 블로그: %s\n- 글: [%s](%s)", blogTitle, articleTitle, articleURL),
		}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("post discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}

	return nil
}
