package fetcher

import (
	"context"

	"github.com/mmcdole/gofeed"
)

type ArticleFetcher func(ctx context.Context, client *Client, articleURL string, item *gofeed.Item) (Article, error)

var providerFetchers = map[int]ArticleFetcher{
	D2BlogID: FetchD2Article,
}

func FetchByBlogID(ctx context.Context, client *Client, blogID int, articleURL string, item *gofeed.Item) (Article, error) {
	if fetcher, ok := providerFetchers[blogID]; ok {
		return fetcher(ctx, client, articleURL, item)
	}
	return FetchArticle(ctx, client, articleURL, item)
}

func RegisterFetcher(blogID int, fetcher ArticleFetcher) {
	providerFetchers[blogID] = fetcher
}
