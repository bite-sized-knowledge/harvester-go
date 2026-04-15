// refetch_rejected retries fetching articles stuck in article_rejected due to
// transient failures (empty title, empty content). If the re-fetch yields a
// non-empty title, the article is moved back into article_queue where it will
// flow through the normal harvest_post pipeline. Rows that still fail are
// left in article_rejected.
//
// Rows rejected for structural reasons (e.g. medium_tag_page_leak, LLM-judged
// low_quality_*) should NOT be in the --reasons list — they won't recover.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"harvester-go/internal/config"
	"harvester-go/internal/database"
	"harvester-go/internal/fetcher"
)

func main() {
	var (
		apply      bool
		limit      int
		reasonsCSV string
		blogID     int
	)
	flag.BoolVar(&apply, "apply", false, "actually update DB (default: dry-run)")
	flag.IntVar(&limit, "limit", 100, "max rows to attempt per run")
	flag.StringVar(&reasonsCSV, "reasons",
		"empty_title,empty_content",
		"comma-separated reject_reason values to retry")
	flag.IntVar(&blogID, "blog-id", 0, "only process this blog_id (0 = all)")
	flag.Parse()

	reasons := splitCSV(reasonsCSV)
	if len(reasons) == 0 {
		fmt.Fprintln(os.Stderr, "reasons list is empty")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config load:", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := database.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to connect database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	client, err := fetcher.NewClient(cfg.ProxyURL, logger)
	if err != nil {
		logger.Error("failed to create http client", "error", err)
		os.Exit(1)
	}

	mode := "dry-run"
	if apply {
		mode = "apply"
	}
	logger.Info("refetch started",
		"mode", mode,
		"reasons", reasons,
		"limit", limit,
		"blog_id", blogID,
	)

	rows, err := db.ListRejectedForRefetch(ctx, reasons, blogID, limit)
	if err != nil {
		logger.Error("list rejected failed", "error", err)
		os.Exit(1)
	}
	logger.Info("loaded rejected rows", "count", len(rows))

	blogs, err := db.GetAllBlogs(ctx)
	if err != nil {
		logger.Error("load blogs failed", "error", err)
		os.Exit(1)
	}
	blogMap := make(map[int]database.Blog, len(blogs))
	for _, b := range blogs {
		blogMap[b.BlogID] = b
	}

	var (
		recovered    int
		stillFailing int
		fetchErrors  int
		missingBlog  int
	)

	for _, row := range rows {
		if ctx.Err() != nil {
			logger.Warn("interrupted", "error", ctx.Err())
			break
		}

		blog, ok := blogMap[row.BlogID]
		if !ok {
			missingBlog++
			logger.Warn("blog not found, skipping", "article_id", row.ArticleID, "blog_id", row.BlogID)
			continue
		}

		article, err := fetcher.FetchByBlogID(ctx, client, blog.BlogID, row.URL, nil, blog.Title)
		if err != nil {
			fetchErrors++
			logger.Warn("fetch failed", "article_id", row.ArticleID, "url", row.URL, "error", err)
			continue
		}

		if strings.TrimSpace(article.Title) == "" {
			stillFailing++
			logger.Info("still empty title",
				"article_id", row.ArticleID,
				"blog_id", row.BlogID,
				"url", row.URL,
				"content_length", article.ContentLength,
			)
			continue
		}

		logger.Info("recovered",
			"article_id", row.ArticleID,
			"blog_id", row.BlogID,
			"url", row.URL,
			"title", article.Title,
			"content_length", article.ContentLength,
		)

		if !apply {
			recovered++
			continue
		}

		// Preserve the original published_at from the rejected row rather than
		// re-deriving it (refetch doesn't have access to the listing context).
		entity := database.ArticleEntity{
			ArticleID:   row.ArticleID,
			BlogID:      blog.BlogID,
			URL:         article.Link,
			Title:       article.Title,
			Thumbnail:   article.Thumbnail,
			Description: article.Description,
			Content:     article.Content,
			ContentLen:  article.ContentLength,
			Lang:        article.Language,
			PublishedAt: row.PublishedAt,
		}

		if err := db.InsertArticle(ctx, entity); err != nil {
			logger.Error("insert into article_queue failed", "article_id", row.ArticleID, "error", err)
			fetchErrors++
			continue
		}

		if err := db.DeleteFromRejected(ctx, row.ArticleID); err != nil {
			logger.Error("delete from article_rejected failed", "article_id", row.ArticleID, "error", err)
			fetchErrors++
			continue
		}

		recovered++
	}

	logger.Info("refetch finished",
		"mode", mode,
		"scanned", len(rows),
		"recovered", recovered,
		"still_failing", stillFailing,
		"fetch_errors", fetchErrors,
		"missing_blog", missingBlog,
	)

	if !apply && recovered > 0 {
		fmt.Fprintln(os.Stderr, "note: dry-run mode. Re-run with --apply to persist changes.")
	}
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
