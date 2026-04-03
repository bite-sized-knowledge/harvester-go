package fetcher

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"

	"harvester-go/internal/database"
	"harvester-go/internal/hasher"
)

const jinaReaderBase = "https://r.jina.ai/"

// FetchJinaArticle fetches an article by rendering it through Jina AI Reader,
// then falls back to standard metadata extraction from the original URL.
func FetchJinaArticle(ctx context.Context, client *Client, articleURL string, item *gofeed.Item) (Article, error) {
	// First, try standard fetch for OG metadata (title, thumbnail, description)
	article, _ := FetchArticle(ctx, client, articleURL, item)

	// Fetch rendered content via Jina
	jinaURL := jinaReaderBase + articleURL
	body, err := client.Get(ctx, jinaURL)
	if err != nil {
		// If Jina fails, return whatever we got from standard fetch
		if article.Title != "" {
			return article, nil
		}
		return Article{}, fmt.Errorf("jina fetch failed: %w", err)
	}

	content := string(body)

	// Extract title from Jina markdown if standard fetch missed it
	if article.Title == "" {
		article.Title = extractJinaTitle(content)
	}

	// Use Jina content as the article content
	if len(content) > len(article.Content) {
		article.Content = content
		article.ContentLength = len(content)
	}

	article.ID = hasher.HashToSha1Base62(articleURL)
	article.Link = articleURL

	return article, nil
}

// DiscoverJinaArticles discovers article URLs from a JS-rendered list page
// by fetching through Jina AI Reader and parsing the markdown for links.
func DiscoverJinaArticles(ctx context.Context, client *Client, blog database.Blog) ([]ListedArticle, error) {
	pageURL := blog.CrawlURL
	if pageURL == "" {
		pageURL = blog.URL
	}
	if pageURL == "" {
		return nil, fmt.Errorf("empty crawl url")
	}

	jinaURL := jinaReaderBase + pageURL
	body, err := client.Get(ctx, jinaURL)
	if err != nil {
		return nil, fmt.Errorf("jina discover fetch: %w", err)
	}

	markdown := string(body)

	baseURL := blog.BaseURL
	if baseURL == "" {
		baseURL = blog.URL
	}

	links := extractLinksFromMarkdown(markdown, baseURL)

	// Filter: keep only links that look like article pages (not navigation/category links)
	results := make([]ListedArticle, 0)
	seen := map[string]struct{}{}
	for _, link := range links {
		if _, ok := seen[link]; ok {
			continue
		}
		if !isArticleLink(link, pageURL) {
			continue
		}
		seen[link] = struct{}{}
		results = append(results, ListedArticle{
			URL:         link,
			PublishedAt: time.Time{},
		})
	}

	return results, nil
}

// urlInParensRe matches all (https://...) patterns in markdown,
// handling nested []() by extracting URLs from parentheses directly.
var urlInParensRe = regexp.MustCompile(`\]\((https?://[^)\s]+)\)`)

// standaloneURLRe matches bare URLs on their own lines
var standaloneURLRe = regexp.MustCompile(`(?m)^(https?://[^\s]+)$`)

func extractLinksFromMarkdown(markdown, baseURL string) []string {
	seen := map[string]struct{}{}
	var links []string

	// Extract URLs from markdown link syntax: ](url)
	for _, m := range urlInParensRe.FindAllStringSubmatch(markdown, -1) {
		if len(m) < 2 {
			continue
		}
		href := strings.TrimSpace(m[1])
		if _, ok := seen[href]; ok {
			continue
		}
		seen[href] = struct{}{}
		resolved := resolveURL(baseURL, href)
		if resolved != "" {
			links = append(links, resolved)
		}
	}

	// Also extract standalone URLs
	for _, m := range standaloneURLRe.FindAllStringSubmatch(markdown, -1) {
		if len(m) < 2 {
			continue
		}
		href := strings.TrimSpace(m[1])
		if _, ok := seen[href]; ok {
			continue
		}
		seen[href] = struct{}{}
		resolved := resolveURL(baseURL, href)
		if resolved != "" {
			links = append(links, resolved)
		}
	}

	return links
}

func isArticleLink(link, pageURL string) bool {
	// Exclude fragment-only links
	if strings.Contains(link, "#") && strings.Split(link, "#")[0] == strings.Split(pageURL, "#")[0] {
		return false
	}

	// Must be from the same domain
	pageDomain := extractDomain(pageURL)
	linkDomain := extractDomain(link)
	if pageDomain != linkDomain {
		return false
	}

	// Exclude static asset extensions
	lower := strings.ToLower(link)
	assetExts := []string{".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico", ".css", ".js", ".pdf", ".zip"}
	for _, ext := range assetExts {
		if strings.HasSuffix(lower, ext) || strings.Contains(lower, ext+"?") {
			return false
		}
	}

	// Exclude common non-article patterns
	excludePatterns := []string{
		"/tags/", "/tag/", "/category/", "/categories/",
		"/page/", "/about", "/contact", "/search",
		"/login", "/signup", "/register",
		"/feed", "/rss", "/atom",
	}
	for _, pattern := range excludePatterns {
		if strings.Contains(lower, pattern) {
			return false
		}
	}

	// Article links typically have more path depth than list pages
	linkPath := extractPath(link)
	pagePath := extractPath(pageURL)
	if linkPath == pagePath || linkPath == "/" || linkPath == "" {
		return false
	}

	return true
}

func extractDomain(rawURL string) string {
	if idx := strings.Index(rawURL, "://"); idx >= 0 {
		rest := rawURL[idx+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			return rest[:slash]
		}
		return rest
	}
	return rawURL
}

func extractPath(rawURL string) string {
	if idx := strings.Index(rawURL, "://"); idx >= 0 {
		rest := rawURL[idx+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			path := rest[slash:]
			if hash := strings.Index(path, "#"); hash >= 0 {
				path = path[:hash]
			}
			if q := strings.Index(path, "?"); q >= 0 {
				path = path[:q]
			}
			return strings.TrimSuffix(path, "/")
		}
	}
	return ""
}

func extractJinaTitle(markdown string) string {
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Title: ") {
			return strings.TrimPrefix(line, "Title: ")
		}
	}
	// Fallback: first markdown heading
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(markdown))
	if err == nil {
		if h1 := doc.Find("h1").First().Text(); h1 != "" {
			return strings.TrimSpace(h1)
		}
	}
	return ""
}
