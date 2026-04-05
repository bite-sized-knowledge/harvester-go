package harvester

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"harvester-go/internal/database"
	"harvester-go/internal/fetcher"
	"harvester-go/internal/hasher"
)

type Runner struct {
	db     *database.DB
	client *fetcher.Client
	parser *gofeed.Parser
	logger *slog.Logger
}

func NewRunner(db *database.DB, client *fetcher.Client, logger *slog.Logger) *Runner {
	return &Runner{
		db:     db,
		client: client,
		parser: gofeed.NewParser(),
		logger: logger,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	blogs, err := r.db.GetAllBlogs(ctx)
	if err != nil {
		return fmt.Errorf("get blogs: %w", err)
	}

	r.logger.Info("starting harvest cycle", "blogs", len(blogs))

	for _, blog := range blogs {
		if err := r.harvestBlog(ctx, blog); err != nil {
			r.logger.Error("blog harvest failed", "blog_id", blog.BlogID, "blog_title", blog.Title, "rss_url", blog.RSSURL, "error", err)
		}
	}

	r.logger.Info("harvest cycle finished")
	return nil
}

func (r *Runner) harvestBlog(ctx context.Context, blog database.Blog) error {
	switch strings.ToUpper(strings.TrimSpace(blog.CrawlType)) {
	case "DEFAULT":
		return r.harvestDefaultBlog(ctx, blog)
	case "MEDIUM":
		return r.harvestMediumBlog(ctx, blog)
	case "JINA":
		return r.harvestJinaBlog(ctx, blog)
	default:
		return r.harvestRSSBlog(ctx, blog)
	}
}

func (r *Runner) harvestRSSBlog(ctx context.Context, blog database.Blog) error {
	body, err := r.client.Get(ctx, blog.RSSURL)
	if err != nil {
		return fmt.Errorf("fetch rss: %w", err)
	}

	feed, err := r.parser.Parse(bytes.NewReader(fetcher.SanitizeXML(body)))
	if err != nil {
		return fmt.Errorf("parse rss: %w", err)
	}

	r.logger.Info("rss loaded", "blog_id", blog.BlogID, "blog_title", blog.Title, "items", len(feed.Items))

	for _, item := range feed.Items {
		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		}
		_, _ = r.ProcessURL(ctx, blog, item.Link, item, publishedAt)
	}

	return nil
}

func (r *Runner) harvestDefaultBlog(ctx context.Context, blog database.Blog) error {
	articles, err := fetcher.DiscoverDefaultArticles(ctx, r.client, blog)
	if err != nil {
		return fmt.Errorf("discover default articles: %w", err)
	}
	r.logger.Info("default blog loaded", "blog_id", blog.BlogID, "blog_title", blog.Title, "items", len(articles))
	for _, article := range articles {
		_, _ = r.ProcessURL(ctx, blog, article.URL, nil, article.PublishedAt)
	}
	return nil
}

func (r *Runner) harvestMediumBlog(ctx context.Context, blog database.Blog) error {
	articles, err := fetcher.DiscoverMediumArticles(ctx, r.client, blog)
	if err != nil {
		return fmt.Errorf("discover medium articles: %w", err)
	}
	r.logger.Info("medium feed loaded", "blog_id", blog.BlogID, "blog_title", blog.Title, "items", len(articles))
	for _, article := range articles {
		_, _ = r.ProcessURL(ctx, blog, article.URL, nil, article.PublishedAt)
	}
	return nil
}

func (r *Runner) harvestJinaBlog(ctx context.Context, blog database.Blog) error {
	articles, err := fetcher.DiscoverJinaArticles(ctx, r.client, blog)
	if err != nil {
		return fmt.Errorf("discover jina articles: %w", err)
	}
	r.logger.Info("jina blog loaded", "blog_id", blog.BlogID, "blog_title", blog.Title, "items", len(articles))
	for _, article := range articles {
		_, _ = r.ProcessURL(ctx, blog, article.URL, nil, article.PublishedAt)
	}
	return nil
}

func (r *Runner) ProcessURL(ctx context.Context, blog database.Blog, rawURL string, item *gofeed.Item, publishedAt time.Time) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	link := normalizeLink(rawURL)
	if link == "" {
		r.logger.Warn("skip item with empty link", "blog_id", blog.BlogID, "blog_title", blog.Title)
		return false, nil
	}

	articleID := hasher.HashToSha1Base62(link)
	exists, err := r.db.IsExistArticle(ctx, articleID)
	if err != nil {
		r.logger.Error("article existence check failed", "article_id", articleID, "url", link, "error", err)
		return false, err
	}
	if exists {
		r.logger.Debug("existing article skipped", "blog_id", blog.BlogID, "article_id", articleID)
		return false, nil
	}

	article, err := r.fetchArticle(ctx, blog.BlogID, link, item)
	if err != nil {
		r.logger.Error("article fetch failed", "blog_id", blog.BlogID, "blog_title", blog.Title, "url", link, "error", err)
		return false, err
	}

	if publishedAt.IsZero() {
		publishedAt = time.Now()
	}

	entity := database.ArticleEntity{
		ArticleID:   articleID,
		BlogID:      blog.BlogID,
		URL:         article.Link,
		Title:       article.Title,
		Thumbnail:   article.Thumbnail,
		Description: article.Description,
		Content:     article.Content,
		ContentLen:  article.ContentLength,
		Lang:        article.Language,
		PublishedAt: publishedAt,
	}

	if err := r.db.InsertArticle(ctx, entity); err != nil {
		r.logger.Error("insert article failed", "article_id", articleID, "url", link, "error", err)
		return false, err
	}

	r.logger.Info("article harvested", "blog_id", blog.BlogID, "blog_title", blog.Title, "article_id", articleID, "title", article.Title)
	return true, nil
}

func (r *Runner) fetchArticle(ctx context.Context, blogID int, link string, item *gofeed.Item) (fetcher.Article, error) {
	return fetcher.FetchByBlogID(ctx, r.client, blogID, link, item)
}

func normalizeLink(value string) string {
	trimmed := strings.TrimSpace(value)
	return strings.TrimSuffix(trimmed, "/")
}
