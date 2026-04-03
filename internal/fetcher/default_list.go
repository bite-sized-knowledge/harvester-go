package fetcher

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"harvester-go/internal/database"
)

type ListedArticle struct {
	URL         string
	PublishedAt time.Time
}

func DiscoverDefaultArticles(ctx context.Context, client *Client, blog database.Blog) ([]ListedArticle, error) {
	pageURL := blog.CrawlURL
	if pageURL == "" {
		pageURL = blog.URL
	}
	if pageURL == "" {
		pageURL = blog.BaseURL
	}
	if pageURL == "" {
		return nil, fmt.Errorf("empty crawl url")
	}

	body, err := client.Get(ctx, pageURL)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse list page: %w", err)
	}

	baseURL := pageURL
	if blog.BaseURL != "" {
		baseURL = blog.BaseURL
	}

	// If link_regex is set, use regex-based extraction from raw HTML
	if blog.LinkRegex != "" {
		return discoverByRegex(string(body), blog)
	}

	results := make([]ListedArticle, 0)
	seen := map[string]struct{}{}
	doc.Find(blog.ArticleSelector).Each(func(_ int, item *goquery.Selection) {
		linkSel := item
		if blog.LinkSelector != "" {
			if selected := item.Find(blog.LinkSelector).First(); selected.Length() > 0 {
				linkSel = selected
			}
		}

		href := strings.TrimSpace(linkSel.AttrOr("href", ""))
		resolved := resolveURL(baseURL, href)
		if resolved == "" {
			return
		}
		if _, ok := seen[resolved]; ok {
			return
		}
		seen[resolved] = struct{}{}

		results = append(results, ListedArticle{
			URL:         resolved,
			PublishedAt: parsePublishedAt(item, blog.PublishSelector, blog.PublishFormat),
		})
	})

	return results, nil
}

// discoverByRegex extracts article URLs using a regex pattern and URL template.
// Capture groups in the regex are substituted into the template as {1}, {2}, etc.
func discoverByRegex(html string, blog database.Blog) ([]ListedArticle, error) {
	re, err := regexp.Compile(blog.LinkRegex)
	if err != nil {
		return nil, fmt.Errorf("compile link_regex: %w", err)
	}

	matches := re.FindAllStringSubmatch(html, -1)
	seen := map[string]struct{}{}
	results := make([]ListedArticle, 0)

	for _, m := range matches {
		articleURL := blog.LinkTemplate
		for i := 1; i < len(m); i++ {
			articleURL = strings.ReplaceAll(articleURL, fmt.Sprintf("{%d}", i), m[i])
		}
		articleURL = strings.TrimSpace(articleURL)
		if articleURL == "" {
			continue
		}
		if _, ok := seen[articleURL]; ok {
			continue
		}
		seen[articleURL] = struct{}{}
		results = append(results, ListedArticle{URL: articleURL})
	}

	return results, nil
}

func parsePublishedAt(item *goquery.Selection, selector, format string) time.Time {
	if selector == "" {
		return time.Time{}
	}
	text := strings.TrimSpace(item.Find(selector).First().Text())
	if text == "" {
		return time.Time{}
	}
	for _, layout := range normalizeLayouts(format) {
		if parsed, err := time.ParseInLocation(layout, text, time.FixedZone("KST", 9*60*60)); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func normalizeLayouts(format string) []string {
	trimmed := strings.TrimSpace(format)
	switch trimmed {
	case "yy.MM.dd":
		return []string{"06.01.02"}
	case "yyyy-MM-dd":
		return []string{"2006-01-02"}
	case "yyyy.MM.dd":
		return []string{"2006.01.02"}
	case "yyyy년 MM월 dd일":
		return []string{"2006년 01월 02일"}
	default:
		return []string{"2006-01-02", "2006.01.02", "06.01.02", "2006년 01월 02일"}
	}
}

func resolveURL(baseURL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return base.ResolveReference(parsed).String()
}
