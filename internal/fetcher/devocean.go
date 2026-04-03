package fetcher

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"harvester-go/internal/database"
)

const devoceanDetailBase = "https://devocean.sk.com/blog/techBoardDetail.do?ID="

var devoceanIDPattern = regexp.MustCompile(`goDetail\(this,'(\d+)'`)

// DiscoverDevoceanArticles extracts article IDs from the Devocean blog list page.
// Devocean uses onclick="goDetail(this,'168151',event)" instead of href links.
func DiscoverDevoceanArticles(ctx context.Context, client *Client, blog database.Blog) ([]ListedArticle, error) {
	pageURL := blog.CrawlURL
	if pageURL == "" {
		pageURL = blog.URL
	}

	body, err := client.Get(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("fetch devocean list: %w", err)
	}

	html := string(body)
	matches := devoceanIDPattern.FindAllStringSubmatch(html, -1)

	seen := map[string]struct{}{}
	results := make([]ListedArticle, 0)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		id := strings.TrimSpace(m[1])
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		results = append(results, ListedArticle{
			URL:         devoceanDetailBase + id,
			PublishedAt: time.Time{},
		})
	}

	return results, nil
}
