package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"

	"harvester-go/internal/hasher"
)

const (
	D2BlogID  = 6
	d2BlogURL = "https://d2.naver.com"
)

var d2IDPattern = regexp.MustCompile(`/((helloworld)|(news))/(\d+)$`)

type d2APIResponse struct {
	PostTitle string `json:"postTitle"`
	PostImage string `json:"postImage"`
	PostHTML  string `json:"postHtml"`
}

func FetchD2Article(ctx context.Context, client *Client, articleURL string, item *gofeed.Item) (Article, error) {
	article, err := FetchArticle(ctx, client, articleURL, item)
	if err != nil {
		return Article{}, err
	}

	match := d2IDPattern.FindStringSubmatch(articleURL)
	if len(match) < 5 {
		return Article{}, fmt.Errorf("invalid D2 article URL: %s", articleURL)
	}

	body, err := client.Get(ctx, fmt.Sprintf("https://d2.naver.com/api/v1/contents/%s", match[4]))
	if err != nil {
		return Article{}, fmt.Errorf("fetch d2 api: %w", err)
	}

	var payload d2APIResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return Article{}, fmt.Errorf("decode d2 api response: %w", err)
	}

	description := article.Description
	if description == "" {
		description = truncate(stripHTML(payload.PostHTML), 150)
	}

	thumbnail := article.Thumbnail
	if payload.PostImage != "" {
		thumbnail = d2BlogURL + payload.PostImage
	}

	content := article.Content
	if payload.PostHTML != "" {
		content = payload.PostHTML
	}

	return Article{
		ID:            hasher.HashToSha1Base62(articleURL),
		Link:          articleURL,
		Title:         firstNonEmpty(article.Title, payload.PostTitle),
		Thumbnail:     thumbnail,
		Description:   description,
		Content:       content,
		ContentLength: len(content),
		Language:      article.Language,
	}, nil
}

func stripHTML(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}
	return strings.Join(strings.Fields(doc.Text()), " ")
}

func truncate(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	return string([]rune(value)[:max])
}
