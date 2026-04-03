package fetcher

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"harvester-go/internal/database"
)

func DiscoverMediumArticles(ctx context.Context, client *Client, blog database.Blog) ([]ListedArticle, error) {
	feedURL, err := BuildMediumFeedURL(blog.CrawlURL)
	if err != nil {
		return nil, err
	}
	body, err := client.Get(ctx, feedURL)
	if err != nil {
		return nil, err
	}
	feed, err := gofeed.NewParser().ParseString(string(SanitizeXML(body)))
	if err != nil {
		return nil, fmt.Errorf("parse medium feed: %w", err)
	}
	items := make([]ListedArticle, 0, len(feed.Items))
	for _, item := range feed.Items {
		published := item.PublishedParsed
		var at time.Time
		if published != nil {
			at = *published
		}
		items = append(items, ListedArticle{URL: item.Link, PublishedAt: at})
	}
	return items, nil
}

func BuildMediumFeedURL(blogLink string) (string, error) {
	trimmed := strings.TrimSuffix(strings.TrimSpace(blogLink), "/")
	if trimmed == "" {
		return "", fmt.Errorf("empty medium blog link")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse medium blog link: %w", err)
	}
	path := strings.TrimSuffix(parsed.Path, "/all")
	trimmed = strings.TrimSuffix(parsed.Scheme+"://"+parsed.Host+path, "/")
	if strings.Contains(trimmed, "medium.com") {
		parts := strings.Split(strings.TrimPrefix(trimmed, parsed.Scheme+"://"+parsed.Host), "/")
		publication := ""
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" || part == "all" {
				continue
			}
			publication = part
			break
		}
		if publication == "" {
			return "", fmt.Errorf("cannot derive medium publication from %s", blogLink)
		}
		return "https://medium.com/feed/" + publication, nil
	}
	return trimmed + "/feed", nil
}
