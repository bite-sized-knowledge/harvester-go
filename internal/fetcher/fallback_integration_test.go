//go:build integration

package fetcher

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// TestFetchWithFallback_WoowahanStatic verifies that a blog with well-behaved
// static HTML (우아한형제 RSS-based blog) still goes through the static path
// and does NOT unnecessarily invoke Jina. Regression guard for the happy path.
func TestFetchWithFallback_WoowahanStatic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	client, err := NewClient("", logger)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Pick a real recent Woowahan post URL.
	url := "https://techblog.woowahan.com/26034"
	article, err := FetchWithFallback(ctx, client, url, nil)
	if err != nil {
		t.Fatalf("FetchWithFallback: %v", err)
	}
	if strings.TrimSpace(article.Title) == "" {
		t.Errorf("empty title from static path: %+v", article)
	}
	if article.ContentLength < fallbackContentMinBytes {
		t.Errorf("content smaller than healthy threshold: %d", article.ContentLength)
	}
	t.Logf("woowahan static OK: title=%q content_length=%d", article.Title, article.ContentLength)
}

// TestFetchWithFallback_DropboxWAF hits a real Dropbox Tech URL to verify
// the end-to-end fallback: static fetch gets the AWS WAF challenge page
// (empty title, ~2KB body) → fallback triggers → Jina returns real content.
// Run with: go test -tags=integration ./internal/fetcher/...
func TestFetchWithFallback_DropboxWAF(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	client, err := NewClient("", logger)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := "https://dropbox.tech/infrastructure/project-schedule-estimation-in-software-development"
	article, err := FetchWithFallback(ctx, client, url, nil)
	if err != nil {
		t.Fatalf("FetchWithFallback: %v", err)
	}

	if !strings.Contains(article.Title, "Project Schedule Estimation") {
		t.Errorf("title not recovered via fallback: %q", article.Title)
	}
	if article.ContentLength < 3000 {
		t.Errorf("content too short after fallback: %d bytes", article.ContentLength)
	}
	t.Logf("recovered via fallback: title=%q content_length=%d", article.Title, article.ContentLength)
}
