package fetcher

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"

	"harvester-go/internal/hasher"
)

type Article struct {
	ID            string
	Link          string
	Title         string
	Thumbnail     string
	Description   string
	Content       string
	ContentLength int
	Language      string
}

func FetchArticle(ctx context.Context, client *Client, url string, item *gofeed.Item) (Article, error) {
	body, err := client.Get(ctx, url)
	if err != nil {
		return Article{}, fmt.Errorf("fetch article page: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return Article{}, fmt.Errorf("parse html: %w", err)
	}

	title := firstNonEmpty(
		metaContent(doc, `meta[property="og:title"]`),
		metaContent(doc, `meta[name="twitter:title"]`),
		safeItemTitle(item),
		strings.TrimSpace(doc.Find("title").First().Text()),
		strings.TrimSpace(doc.Find("h1").First().Text()),
	)

	thumbnail := firstNonEmpty(
		metaContent(doc, `meta[property="og:image"]`),
		metaContent(doc, `meta[name="twitter:image"]`),
	)

	description := firstNonEmpty(
		metaContent(doc, `meta[property="og:description"]`),
		metaContent(doc, `meta[name="twitter:description"]`),
		safeItemContent(item),
	)

	language := firstNonEmpty(
		metaContent(doc, `meta[property="og:locale"]`),
		strings.TrimSpace(doc.Find("html").AttrOr("lang", "")),
		"unknown",
	)

	content := string(body)

	return Article{
		ID:            hasher.HashToSha1Base62(url),
		Link:          url,
		Title:         title,
		Thumbnail:     thumbnail,
		Description:   description,
		Content:       content,
		ContentLength: len(content),
		Language:      language,
	}, nil
}

func metaContent(doc *goquery.Document, selector string) string {
	return strings.TrimSpace(doc.Find(selector).First().AttrOr("content", ""))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func safeItemTitle(item *gofeed.Item) string {
	if item == nil {
		return ""
	}
	return strings.TrimSpace(item.Title)
}

func safeItemContent(item *gofeed.Item) string {
	if item == nil {
		return ""
	}
	return strings.TrimSpace(item.Content)
}
