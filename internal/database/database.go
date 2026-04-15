package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"

	"harvester-go/internal/config"
)

type DB struct {
	pool   *sql.DB
	logger *slog.Logger
}

type ArticleEntity struct {
	ArticleID   string
	BlogID      int
	URL         string
	Title       string
	Thumbnail   string
	Description string
	Content     string
	ContentLen  int
	Lang        string
	PublishedAt time.Time
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*DB, error) {
	tlsMode := "skip-verify"
	if caPath := os.Getenv("DB_TLS_CA"); caPath != "" {
		rootCertPool := x509.NewCertPool()
		pem, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("read DB TLS CA: %w", err)
		}
		if ok := rootCertPool.AppendCertsFromPEM(pem); !ok {
			return nil, fmt.Errorf("failed to parse DB TLS CA certificate")
		}
		if err := mysql.RegisterTLSConfig("mysql-tls", &tls.Config{
			RootCAs:            rootCertPool,
			InsecureSkipVerify: true, // skip hostname check (self-signed CN doesn't match Docker service name)
			VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
				cert, err := x509.ParseCertificate(rawCerts[0])
				if err != nil {
					return fmt.Errorf("parse peer cert: %w", err)
				}
				_, err = cert.Verify(x509.VerifyOptions{Roots: rootCertPool})
				return err
			},
		}); err != nil {
			return nil, fmt.Errorf("register TLS config: %w", err)
		}
		tlsMode = "mysql-tls"
		logger.Info("MySQL TLS: using CA certificate verification")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&tls=%s", cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName, tlsMode)

	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(5 * time.Minute)

	if err := pool.PingContext(ctx); err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return &DB{pool: pool, logger: logger}, nil
}

func (d *DB) Close() error {
	return d.pool.Close()
}

func (d *DB) GetAllBlogs(ctx context.Context) ([]Blog, error) {
	rows, err := d.pool.QueryContext(ctx, `
		SELECT
			blog_id,
			COALESCE(title, ''),
			COALESCE(url, ''),
			COALESCE(rss_url, ''),
			COALESCE(crawl_type, 'RSS'),
			COALESCE(crawl_url, ''),
			COALESCE(external_source, ''),
			COALESCE(external_id, 0),
			COALESCE(base_url, ''),
			COALESCE(article_selector, ''),
			COALESCE(title_selector, ''),
			COALESCE(link_selector, ''),
			COALESCE(thumbnail_selector, ''),
			COALESCE(publish_selector, ''),
			COALESCE(publish_format, ''),
			COALESCE(publish_type, ''),
			COALESCE(inner_publish_selector, ''),
			COALESCE(pagination_type, ''),
			COALESCE(page_url_pattern, ''),
			COALESCE(next_page_selector, ''),
			COALESCE(max_pages, 0),
			COALESCE(link_regex, ''),
			COALESCE(link_template, ''),
			is_active
		FROM blog
		WHERE is_active = TRUE
		ORDER BY blog_id`)
	if err != nil {
		return nil, fmt.Errorf("query blogs: %w", err)
	}
	defer rows.Close()

	blogs := make([]Blog, 0)
	for rows.Next() {
		var b Blog
		if err := rows.Scan(
			&b.BlogID,
			&b.Title,
			&b.URL,
			&b.RSSURL,
			&b.CrawlType,
			&b.CrawlURL,
			&b.ExternalSource,
			&b.ExternalID,
			&b.BaseURL,
			&b.ArticleSelector,
			&b.TitleSelector,
			&b.LinkSelector,
			&b.ThumbnailSelector,
			&b.PublishSelector,
			&b.PublishFormat,
			&b.PublishType,
			&b.InnerPublishSelector,
			&b.PaginationType,
			&b.PageURLPattern,
			&b.NextPageSelector,
			&b.MaxPages,
			&b.LinkRegex,
			&b.LinkTemplate,
			&b.IsActive,
		); err != nil {
			return nil, fmt.Errorf("scan blog: %w", err)
		}
		blogs = append(blogs, b)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate blogs: %w", err)
	}

	return blogs, nil
}

func (d *DB) UpsertBlog(ctx context.Context, blog BlogUpsert) error {
	_, err := d.pool.ExecContext(ctx, `
		INSERT INTO blog (
			blog_id, title, url, rss_url, favicon,
			crawl_type, crawl_url, external_source, external_id,
			base_url, article_selector, title_selector, link_selector, thumbnail_selector,
			publish_selector, publish_format, publish_type, inner_publish_selector,
			pagination_type, page_url_pattern, next_page_selector, max_pages
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			title = VALUES(title),
			url = VALUES(url),
			rss_url = VALUES(rss_url),
			favicon = VALUES(favicon),
			crawl_type = VALUES(crawl_type),
			crawl_url = VALUES(crawl_url),
			external_source = VALUES(external_source),
			external_id = VALUES(external_id),
			base_url = VALUES(base_url),
			article_selector = VALUES(article_selector),
			title_selector = VALUES(title_selector),
			link_selector = VALUES(link_selector),
			thumbnail_selector = VALUES(thumbnail_selector),
			publish_selector = VALUES(publish_selector),
			publish_format = VALUES(publish_format),
			publish_type = VALUES(publish_type),
			inner_publish_selector = VALUES(inner_publish_selector),
			pagination_type = VALUES(pagination_type),
			page_url_pattern = VALUES(page_url_pattern),
			next_page_selector = VALUES(next_page_selector),
			max_pages = VALUES(max_pages),
			updated_at = CURRENT_TIMESTAMP`,
		blog.BlogID, blog.Title, blog.URL, blog.RSSURL, blog.Favicon,
		blog.CrawlType, blog.CrawlURL, blog.ExternalSource, blog.ExternalID,
		blog.BaseURL, blog.ArticleSelector, blog.TitleSelector, blog.LinkSelector, blog.ThumbnailSelector,
		blog.PublishSelector, blog.PublishFormat, blog.PublishType, blog.InnerPublishSelector,
		blog.PaginationType, blog.PageURLPattern, blog.NextPageSelector, nullIfZero(blog.MaxPages),
	)
	if err != nil {
		return fmt.Errorf("upsert blog blog_id=%d: %w", blog.BlogID, err)
	}
	return nil
}

func (d *DB) HasCanonicalBlogConflict(ctx context.Context, url string) (bool, error) {
	normalized := strings.TrimSuffix(strings.TrimSpace(url), "/")
	if normalized == "" {
		return false, nil
	}
	var count int
	err := d.pool.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM blog
		WHERE (external_source IS NULL OR external_source != 'newcodes')
		  AND url IS NOT NULL
		  AND url != ''
		  AND (? = TRIM(TRAILING '/' FROM url)
		       OR ? LIKE CONCAT(TRIM(TRAILING '/' FROM url), '%')
		       OR TRIM(TRAILING '/' FROM url) LIKE CONCAT(?, '%'))`,
		normalized, normalized, normalized,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check canonical blog conflict: %w", err)
	}
	return count > 0, nil
}

func nullIfZero(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func (d *DB) IsExistArticle(ctx context.Context, articleID string) (bool, error) {
	var exists bool
	if err := d.pool.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM article WHERE article_id = ?) OR EXISTS(SELECT 1 FROM article_queue WHERE article_id = ?)",
		articleID, articleID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("query article existence: %w", err)
	}
	return exists, nil
}

func (d *DB) InsertArticle(ctx context.Context, article ArticleEntity) error {
	article.URL = clampString(article.URL, 500)
	article.Title = clampString(article.Title, 255)
	article.Thumbnail = clampString(article.Thumbnail, 500)
	article.Description = clampString(article.Description, 1000)
	article.Lang = clampString(article.Lang, 10)

	_, err := d.pool.ExecContext(
		ctx,
		"INSERT INTO article_queue (article_id, blog_id, url, title, thumbnail, description, content, content_length, lang, published_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		article.ArticleID,
		article.BlogID,
		article.URL,
		article.Title,
		article.Thumbnail,
		article.Description,
		article.Content,
		article.ContentLen,
		article.Lang,
		article.PublishedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "Error 1062") {
			return nil
		}
		return fmt.Errorf("insert article_queue article_id=%s: %w", article.ArticleID, err)
	}

	d.logger.Debug("article inserted", "article_id", article.ArticleID, "blog_id", article.BlogID)
	return nil
}

// RejectedRow is the subset of article_rejected needed to retry a fetch.
type RejectedRow struct {
	ArticleID    string
	BlogID       int
	URL          string
	PublishedAt  time.Time
	RejectReason string
}

// ListRejectedForRefetch returns rows from article_rejected matching any of
// the given reason strings. Pass blogID=0 to include all blogs.
func (d *DB) ListRejectedForRefetch(ctx context.Context, reasons []string, blogID, limit int) ([]RejectedRow, error) {
	if len(reasons) == 0 {
		return nil, fmt.Errorf("reasons list is empty")
	}
	placeholders := make([]string, len(reasons))
	args := make([]any, 0, len(reasons)+2)
	for i, r := range reasons {
		placeholders[i] = "?"
		args = append(args, r)
	}
	query := fmt.Sprintf(
		`SELECT article_id, blog_id, COALESCE(url,''), COALESCE(published_at, CURRENT_TIMESTAMP), reject_reason
		 FROM article_rejected
		 WHERE reject_reason IN (%s)`,
		strings.Join(placeholders, ","),
	)
	if blogID > 0 {
		query += " AND blog_id = ?"
		args = append(args, blogID)
	}
	query += " ORDER BY rejected_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := d.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query article_rejected: %w", err)
	}
	defer rows.Close()

	out := make([]RejectedRow, 0, limit)
	for rows.Next() {
		var r RejectedRow
		if err := rows.Scan(&r.ArticleID, &r.BlogID, &r.URL, &r.PublishedAt, &r.RejectReason); err != nil {
			return nil, fmt.Errorf("scan article_rejected row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate article_rejected rows: %w", err)
	}
	return out, nil
}

// DeleteFromRejected removes a single row from article_rejected by article_id.
func (d *DB) DeleteFromRejected(ctx context.Context, articleID string) error {
	_, err := d.pool.ExecContext(ctx, "DELETE FROM article_rejected WHERE article_id = ?", articleID)
	if err != nil {
		return fmt.Errorf("delete article_rejected article_id=%s: %w", articleID, err)
	}
	return nil
}

func clampString(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}

	runes := []rune(value)
	return string(runes[:max])
}
