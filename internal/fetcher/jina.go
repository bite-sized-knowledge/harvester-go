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

// enrichViaJina fetches the given URL through Jina Reader and merges the
// result into `base` (which may hold OG metadata from a prior static fetch).
// It is only called from FetchWithFallback when the static fetch looks
// broken — never as a first-line fetcher.
func enrichViaJina(ctx context.Context, client *Client, articleURL string, base Article) (Article, error) {
	jinaURL := jinaReaderBase + articleURL
	body, err := client.Get(ctx, jinaURL)
	if err != nil {
		// If Jina fails, return whatever the static fetch produced.
		if base.Title != "" {
			return base, nil
		}
		return Article{}, fmt.Errorf("jina fetch failed: %w", err)
	}

	content := string(body)

	if jinaTitle := extractJinaTitle(content); jinaTitle != "" && len(jinaTitle) > len(base.Title) {
		base.Title = jinaTitle
	}

	if len(content) > len(base.Content) {
		base.Content = content
		base.ContentLength = len(content)
	}

	base.ID = hasher.HashToSha1Base62(articleURL)
	base.Link = articleURL

	return base, nil
}

// Minimum "healthy" thresholds for a static fetch. Below these we fall back
// to Jina. The 2500-byte content threshold is tuned against the Dropbox WAF
// challenge page (2007 bytes); any legitimate article page exceeds this.
const (
	fallbackContentMinBytes = 2500
)

// looksHealthy returns true when a static-fetched article is usable as-is
// and does not need a Jina fallback.
func looksHealthy(a Article, blogTitle string) bool {
	title := strings.TrimSpace(a.Title)
	if title == "" {
		return false
	}
	if blogTitle != "" && strings.EqualFold(title, strings.TrimSpace(blogTitle)) {
		return false
	}
	if a.ContentLength < fallbackContentMinBytes {
		return false
	}
	return true
}

// FetchWithFallback is the default article fetcher used for most blogs.
// It tries a plain static fetch first; if the result looks broken
// (empty title or suspiciously short body — typical of WAF challenge pages
// or JS-rendered SPAs), it automatically retries through Jina Reader.
//
// The fallback is opportunistic: we keep whatever metadata the static
// fetch managed to extract (often OG tags survive even when the body is
// a challenge page) and let Jina supply the real content and title.
func FetchWithFallback(ctx context.Context, client *Client, articleURL string, item *gofeed.Item, blogTitle string) (Article, error) {
	article, err := FetchArticle(ctx, client, articleURL, item)
	if err == nil && looksHealthy(article, blogTitle) {
		return article, nil
	}
	// Static failed or looks broken — retry via Jina, reusing the partial
	// article we already have so Jina only adds content/title.
	return enrichViaJina(ctx, client, articleURL, article)
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
		if !IsArticleLink(link, pageURL) {
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

	collectMatches := func(matches [][]string) {
		for _, m := range matches {
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
	}

	collectMatches(urlInParensRe.FindAllStringSubmatch(markdown, -1))
	collectMatches(standaloneURLRe.FindAllStringSubmatch(markdown, -1))

	return links
}

var assetExts = []string{".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico", ".css", ".js", ".pdf", ".zip"}

var excludePatterns = []string{
	"/tags/", "/tag/", "/tagged/", "/category/", "/categories/",
	"/page/", "/about", "/contact", "/search",
	"/login", "/signup", "/register",
	"/feed", "/rss", "/atom",
	"/legal/", "/help/", "/accessibility", "/psettings/",
}

func IsArticleLink(link, pageURL string) bool {
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

	lower := strings.ToLower(link)
	for _, ext := range assetExts {
		if strings.HasSuffix(lower, ext) || strings.Contains(lower, ext+"?") {
			return false
		}
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
