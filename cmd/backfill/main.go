package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"

	"harvester-go/internal/config"
	"harvester-go/internal/database"
	"harvester-go/internal/fetcher"
	worker "harvester-go/internal/harvester"
	"harvester-go/internal/sitemap"
)

type repeatedFlag []string

func (r *repeatedFlag) String() string { return strings.Join(*r, ",") }
func (r *repeatedFlag) Set(value string) error {
	*r = append(*r, strings.TrimSpace(value))
	return nil
}

func main() {
	var blogID int
	var sourceURL string
	var sourceKind string
	var sinceRaw string
	var maxURLs int
	var startID int
	var endID int
	var includePrefixes repeatedFlag
	var excludeSubstrings repeatedFlag

	flag.IntVar(&blogID, "blog-id", 0, "blog_id in DB")
	flag.StringVar(&sourceURL, "source-url", "", "root sitemap or archive XML URL")
	flag.StringVar(&sourceKind, "source-kind", "sitemap", "source type: sitemap, feed, html, or kakao-range")
	flag.StringVar(&sinceRaw, "since", "2024-03-28", "collect URLs with lastmod on/after this date (YYYY-MM-DD)")
	flag.IntVar(&maxURLs, "max-urls", 0, "optional max URL count after filtering")
	flag.IntVar(&startID, "start-id", 0, "start post id for range-based source")
	flag.IntVar(&endID, "end-id", 0, "end post id for range-based source")
	flag.Var(&includePrefixes, "include-prefix", "only include URLs starting with this prefix (repeatable)")
	flag.Var(&excludeSubstrings, "exclude-substring", "exclude URLs containing this substring (repeatable)")
	flag.Parse()

	if blogID <= 0 {
		panic("-blog-id is required")
	}

	if strings.TrimSpace(sourceURL) == "" && strings.ToLower(strings.TrimSpace(sourceKind)) != "kakao-range" {
		panic("-source-url is required unless source-kind is kakao-range")
	}

	since, err := time.Parse("2006-01-02", strings.TrimSpace(sinceRaw))
	if err != nil {
		panic(fmt.Errorf("invalid -since: %w", err))
	}

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := database.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to connect database", "error", err)
		return
	}
	defer db.Close()

	blogs, err := db.GetAllBlogs(ctx)
	if err != nil {
		logger.Error("failed to get blogs", "error", err)
		return
	}

	blog, ok := findBlog(blogs, blogID)
	if !ok {
		logger.Error("blog not found", "blog_id", blogID)
		return
	}

	client, err := fetcher.NewClient(cfg.ProxyURL, logger)
	if err != nil {
		logger.Error("failed to create http client", "error", err)
		return
	}

	runner := worker.NewRunner(db, client, logger)
	parsedKind := strings.ToLower(strings.TrimSpace(sourceKind))
	if parsedKind != "sitemap" && parsedKind != "feed" && parsedKind != "html" && parsedKind != "kakao-range" {
		logger.Error("unsupported source kind", "source_kind", sourceKind)
		return
	}

	inserted := 0
	failures := 0
	discovered := 0
	filteredCount := 0

	switch parsedKind {
	case "sitemap":
		items, err := sitemap.Discover(ctx, client, sourceURL)
		if err != nil {
			logger.Error("failed to discover sitemap URLs", "source_url", sourceURL, "error", err)
			return
		}
		discovered = len(items)
		filtered := filterURLs(items, since, includePrefixes, excludeSubstrings)
		if maxURLs > 0 && len(filtered) > maxURLs {
			filtered = filtered[:maxURLs]
		}
		filteredCount = len(filtered)

		for _, item := range filtered {
			select {
			case <-ctx.Done():
				logger.Warn("backfill canceled", "inserted", inserted, "failures", failures)
				return
			default:
			}

			wasInserted, err := runner.ProcessURL(ctx, blog, item.Loc, nil, item.LastMod)
			if err != nil {
				failures++
				continue
			}
			if wasInserted {
				inserted++
			}
		}
	case "feed":
		body, err := client.Get(ctx, sourceURL)
		if err != nil {
			logger.Error("failed to fetch feed", "source_url", sourceURL, "error", err)
			return
		}
		parser := gofeed.NewParser()
		feed, err := parser.Parse(bytes.NewReader(body))
		if err != nil {
			logger.Error("failed to parse feed", "source_url", sourceURL, "error", err)
			return
		}
		discovered = len(feed.Items)
		items := make([]feedItem, 0, len(feed.Items))
		for _, item := range feed.Items {
			publishedAt := time.Time{}
			if item.PublishedParsed != nil {
				publishedAt = *item.PublishedParsed
			}
			items = append(items, feedItem{Item: item, PublishedAt: publishedAt})
		}
		filtered := filterFeedItems(items, since, includePrefixes, excludeSubstrings)
		if maxURLs > 0 && len(filtered) > maxURLs {
			filtered = filtered[:maxURLs]
		}
		filteredCount = len(filtered)

		for _, item := range filtered {
			select {
			case <-ctx.Done():
				logger.Warn("backfill canceled", "inserted", inserted, "failures", failures)
				return
			default:
			}

			wasInserted, err := runner.ProcessURL(ctx, blog, item.Item.Link, item.Item, item.PublishedAt)
			if err != nil {
				failures++
				continue
			}
			if wasInserted {
				inserted++
			}
		}
	case "html":
		body, err := client.Get(ctx, sourceURL)
		if err != nil {
			logger.Error("failed to fetch html source", "source_url", sourceURL, "error", err)
			return
		}
		links, err := extractHTMLLinks(body, sourceURL)
		if err != nil {
			logger.Error("failed to extract html links", "source_url", sourceURL, "error", err)
			return
		}
		discovered = len(links)
		items := make([]sitemap.URLItem, 0, len(links))
		for _, link := range links {
			items = append(items, sitemap.URLItem{Loc: link})
		}
		filtered := filterURLs(items, since, includePrefixes, excludeSubstrings)
		if maxURLs > 0 && len(filtered) > maxURLs {
			filtered = filtered[:maxURLs]
		}
		filteredCount = len(filtered)

		for _, item := range filtered {
			select {
			case <-ctx.Done():
				logger.Warn("backfill canceled", "inserted", inserted, "failures", failures)
				return
			default:
			}

			wasInserted, err := runner.ProcessURL(ctx, blog, item.Loc, nil, time.Time{})
			if err != nil {
				failures++
				continue
			}
			if wasInserted {
				inserted++
			}
		}
	case "kakao-range":
		if startID <= 0 || endID <= 0 || endID < startID {
			logger.Error("start-id and end-id are required for kakao-range", "start_id", startID, "end_id", endID)
			return
		}

		discovered = endID - startID + 1
		for id := startID; id <= endID; id++ {
			select {
			case <-ctx.Done():
				logger.Warn("backfill canceled", "inserted", inserted, "failures", failures)
				return
			default:
			}

			targetURL := fmt.Sprintf("https://tech.kakao.com/posts/%d", id)
			if len(includePrefixes) > 0 && !hasAnyPrefix(targetURL, includePrefixes) {
				continue
			}
			if containsAny(targetURL, excludeSubstrings) {
				continue
			}

			body, err := client.Get(ctx, targetURL)
			if err != nil {
				failures++
				continue
			}

			publishedAt, ok := extractKakaoPublishedAt(body)
			if !ok {
				failures++
				continue
			}
			if publishedAt.Before(since) {
				continue
			}
			filteredCount++

			wasInserted, err := runner.ProcessURL(ctx, blog, targetURL, nil, publishedAt)
			if err != nil {
				failures++
				continue
			}
			if wasInserted {
				inserted++
			}
		}
	}

	logger.Info("backfill finished", "blog_id", blog.BlogID, "blog_title", blog.Title, "source_url", sourceURL, "source_kind", parsedKind, "discovered", discovered, "filtered", filteredCount, "inserted", inserted, "failures", failures)
}

type feedItem struct {
	Item        *gofeed.Item
	PublishedAt time.Time
}

func findBlog(blogs []database.Blog, blogID int) (database.Blog, bool) {
	for _, blog := range blogs {
		if blog.BlogID == blogID {
			return blog, true
		}
	}
	return database.Blog{}, false
}

func filterURLs(items []sitemap.URLItem, since time.Time, includePrefixes, excludeSubstrings []string) []sitemap.URLItem {
	filtered := make([]sitemap.URLItem, 0, len(items))
	for _, item := range items {
		loc := strings.TrimSpace(item.Loc)
		if loc == "" {
			continue
		}
		if !item.LastMod.IsZero() && item.LastMod.Before(since) {
			continue
		}
		if len(includePrefixes) > 0 && !hasAnyPrefix(loc, includePrefixes) {
			continue
		}
		if containsAny(loc, excludeSubstrings) {
			continue
		}
		filtered = append(filtered, item)
	}

	slices.SortFunc(filtered, func(a, b sitemap.URLItem) int {
		if a.LastMod.Equal(b.LastMod) {
			return strings.Compare(a.Loc, b.Loc)
		}
		if a.LastMod.IsZero() {
			return 1
		}
		if b.LastMod.IsZero() {
			return -1
		}
		if a.LastMod.Before(b.LastMod) {
			return -1
		}
		return 1
	})

	return filtered
}

func filterFeedItems(items []feedItem, since time.Time, includePrefixes, excludeSubstrings []string) []feedItem {
	filtered := make([]feedItem, 0, len(items))
	for _, item := range items {
		if item.Item == nil {
			continue
		}
		link := strings.TrimSpace(item.Item.Link)
		if link == "" {
			continue
		}
		if !item.PublishedAt.IsZero() && item.PublishedAt.Before(since) {
			continue
		}
		if len(includePrefixes) > 0 && !hasAnyPrefix(link, includePrefixes) {
			continue
		}
		if containsAny(link, excludeSubstrings) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func containsAny(value string, substrings []string) bool {
	for _, needle := range substrings {
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func extractHTMLLinks(body []byte, pageURL string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	links := make([]string, 0)
	doc.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
		href := strings.TrimSpace(sel.AttrOr("href", ""))
		if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
			return
		}
		parsed, err := url.Parse(href)
		if err != nil {
			return
		}
		resolved := base.ResolveReference(parsed).String()
		if _, ok := seen[resolved]; ok {
			return
		}
		seen[resolved] = struct{}{}
		links = append(links, resolved)
	})

	return links, nil
}

var kakaoDatePattern = regexp.MustCompile(`"(20\d\d\.\d\d\.\d\d)(?:\s+(\d\d:\d\d:\d\d))?"`)

func extractKakaoPublishedAt(body []byte) (time.Time, bool) {
	matches := kakaoDatePattern.FindAllStringSubmatch(string(body), -1)
	for _, match := range matches {
		datePart := match[1]
		timePart := "00:00:00"
		if len(match) > 2 && strings.TrimSpace(match[2]) != "" {
			timePart = match[2]
		}
		parsed, err := time.ParseInLocation("2006.01.02 15:04:05", datePart+" "+timePart, time.FixedZone("KST", 9*60*60))
		if err != nil {
			continue
		}
		if parsed.Year() >= 2020 {
			return parsed, true
		}
	}
	return time.Time{}, false
}
