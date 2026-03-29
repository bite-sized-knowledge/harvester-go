package sitemap

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"harvester-go/internal/fetcher"
)

type URLItem struct {
	Loc     string
	LastMod time.Time
}

type urlSet struct {
	URLs []urlEntry `xml:"url"`
}

type urlEntry struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod"`
}

type sitemapIndex struct {
	Sitemaps []sitemapEntry `xml:"sitemap"`
}

type sitemapEntry struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod"`
}

func Discover(ctx context.Context, client *fetcher.Client, rootURL string) ([]URLItem, error) {
	visited := map[string]struct{}{}
	collected := map[string]URLItem{}

	if err := discoverRecursive(ctx, client, rootURL, visited, collected); err != nil {
		return nil, err
	}

	items := make([]URLItem, 0, len(collected))
	for _, item := range collected {
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].LastMod.Equal(items[j].LastMod) {
			return items[i].Loc < items[j].Loc
		}
		if items[i].LastMod.IsZero() {
			return false
		}
		if items[j].LastMod.IsZero() {
			return true
		}
		return items[i].LastMod.Before(items[j].LastMod)
	})

	return items, nil
}

func discoverRecursive(ctx context.Context, client *fetcher.Client, targetURL string, visited map[string]struct{}, collected map[string]URLItem) error {
	if _, ok := visited[targetURL]; ok {
		return nil
	}
	visited[targetURL] = struct{}{}

	body, err := client.Get(ctx, targetURL)
	if err != nil {
		return fmt.Errorf("fetch sitemap %s: %w", targetURL, err)
	}

	body, err = maybeGunzip(body, targetURL)
	if err != nil {
		return fmt.Errorf("decode sitemap %s: %w", targetURL, err)
	}

	if strings.Contains(string(body), "<sitemapindex") {
		var idx sitemapIndex
		if err := xml.Unmarshal(body, &idx); err != nil {
			return fmt.Errorf("unmarshal sitemap index %s: %w", targetURL, err)
		}
		for _, entry := range idx.Sitemaps {
			loc := strings.TrimSpace(entry.Loc)
			if loc == "" {
				continue
			}
			if err := discoverRecursive(ctx, client, loc, visited, collected); err != nil {
				return err
			}
		}
		return nil
	}

	var set urlSet
	if err := xml.Unmarshal(body, &set); err != nil {
		return fmt.Errorf("unmarshal urlset %s: %w", targetURL, err)
	}

	for _, entry := range set.URLs {
		loc := strings.TrimSpace(entry.Loc)
		if loc == "" {
			continue
		}
		collected[loc] = URLItem{
			Loc:     loc,
			LastMod: parseLastMod(entry.LastMod),
		}
	}

	return nil
}

func maybeGunzip(body []byte, targetURL string) ([]byte, error) {
	if len(body) < 2 {
		return body, nil
	}
	if !strings.HasSuffix(strings.ToLower(targetURL), ".gz") && !(body[0] == 0x1f && body[1] == 0x8b) {
		return body, nil
	}

	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	decoded, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func parseLastMod(raw string) time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05-07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed
		}
	}

	return time.Time{}
}
