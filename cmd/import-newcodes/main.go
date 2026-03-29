package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"harvester-go/internal/config"
	"harvester-go/internal/database"
	"harvester-go/internal/fetcher"
)

type corporationsResponse struct {
	Content       []corporationSummary `json:"content"`
	TotalElements int                  `json:"totalElements"`
}

type corporationSummary struct {
	ID int `json:"id"`
}

type corporationDetail struct {
	ID                   int    `json:"id"`
	Name                 string `json:"name"`
	BlogLink             string `json:"blogLink"`
	BlogType             string `json:"blogType"`
	BaseURL              string `json:"baseUrl"`
	Article              string `json:"article"`
	Title                string `json:"title"`
	Link                 string `json:"link"`
	Thumbnail            string `json:"thumbnail"`
	Publish              string `json:"publish"`
	PublishFormat        string `json:"publishFormat"`
	PublishType          string `json:"publishType"`
	InnerPublishSelector string `json:"innerPublishSelector"`
	PaginationType       string `json:"paginationType"`
	PageURLPattern       string `json:"pageUrlPattern"`
	NextPageSelector     string `json:"nextPageSelector"`
	MaxPages             int    `json:"maxPages"`
	MediumBlog           bool   `json:"mediumBlog"`
	EffectiveLogoURL     string `json:"effectiveLogoUrl"`
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	ctx := context.Background()
	db, err := database.New(ctx, cfg, logger)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	httpClient := &http.Client{Timeout: 30 * time.Second}
	list, err := fetchList(httpClient)
	if err != nil {
		panic(err)
	}

	imported := 0
	for _, item := range list.Content {
		detail, err := fetchDetail(httpClient, item.ID)
		if err != nil {
			logger.Warn("skip corporation detail", "id", item.ID, "error", err)
			continue
		}
		blog := mapCorporation(detail)
		if conflict, err := db.HasCanonicalBlogConflict(ctx, blog.URL); err == nil && conflict {
			logger.Info("skip imported duplicate provider", "id", item.ID, "name", detail.Name, "url", blog.URL)
			continue
		}
		if err := db.UpsertBlog(ctx, blog); err != nil {
			logger.Warn("upsert failed", "id", item.ID, "name", detail.Name, "error", err)
			continue
		}
		imported++
		time.Sleep(700 * time.Millisecond)
	}
	logger.Info("newcodes import finished", "total", len(list.Content), "imported", imported)
}

func fetchList(client *http.Client) (corporationsResponse, error) {
	var result corporationsResponse
	req, _ := http.NewRequest(http.MethodGet, "https://newcodes.net/api/corporations?page=0&size=100", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}

func fetchDetail(client *http.Client, id int) (corporationDetail, error) {
	var result corporationDetail
	for attempt := 0; attempt < 3; attempt++ {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://newcodes.net/api/corporations/%d", id), nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			defer resp.Body.Close()
			if resp.StatusCode < 400 {
				if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
					return result, nil
				}
			}
		}
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}
	return result, fmt.Errorf("failed to fetch detail %d", id)
}

func mapCorporation(c corporationDetail) database.BlogUpsert {
	crawlType := "DEFAULT"
	if c.BlogType == "MEDIUM" || c.MediumBlog || isMediumLikeURL(c.BlogLink) || isMediumLikeURL(c.BaseURL) {
		crawlType = "MEDIUM"
	}
	primaryURL := c.BlogLink
	if primaryURL == "" {
		primaryURL = c.BaseURL
	}
	rssURL := ""
	if crawlType == "MEDIUM" {
		if feedURL, err := fetcher.BuildMediumFeedURL(primaryURL); err == nil {
			rssURL = feedURL
		}
	}
	return database.BlogUpsert{
		BlogID:               1000 + c.ID,
		Title:                c.Name,
		URL:                  primaryURL,
		RSSURL:               rssURL,
		Favicon:              c.EffectiveLogoURL,
		CrawlType:            crawlType,
		CrawlURL:             primaryURL,
		ExternalSource:       "newcodes",
		ExternalID:           c.ID,
		BaseURL:              c.BaseURL,
		ArticleSelector:      c.Article,
		TitleSelector:        c.Title,
		LinkSelector:         c.Link,
		ThumbnailSelector:    c.Thumbnail,
		PublishSelector:      c.Publish,
		PublishFormat:        c.PublishFormat,
		PublishType:          c.PublishType,
		InnerPublishSelector: c.InnerPublishSelector,
		PaginationType:       c.PaginationType,
		PageURLPattern:       c.PageURLPattern,
		NextPageSelector:     c.NextPageSelector,
		MaxPages:             c.MaxPages,
	}
}

func isMediumLikeURL(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "medium.com") ||
		strings.Contains(trimmed, "netflixtechblog.com") ||
		strings.Contains(trimmed, "techblog.yogiyo.co.kr") ||
		strings.Contains(trimmed, "techblog.gccompany.co.kr") ||
		strings.Contains(trimmed, "techblog.lotteon.com")
}
