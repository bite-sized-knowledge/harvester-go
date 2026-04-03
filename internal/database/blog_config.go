package database

type Blog struct {
	BlogID               int
	Title                string
	URL                  string
	RSSURL               string
	CrawlType            string
	CrawlURL             string
	ExternalSource       string
	ExternalID           int
	BaseURL              string
	ArticleSelector      string
	TitleSelector        string
	LinkSelector         string
	ThumbnailSelector    string
	PublishSelector      string
	PublishFormat        string
	PublishType          string
	InnerPublishSelector string
	PaginationType       string
	PageURLPattern       string
	NextPageSelector     string
	MaxPages             int
	LinkRegex            string
	LinkTemplate         string
}

type BlogUpsert struct {
	BlogID               int
	Title                string
	URL                  string
	RSSURL               string
	CrawlType            string
	CrawlURL             string
	ExternalSource       string
	ExternalID           int
	BaseURL              string
	ArticleSelector      string
	TitleSelector        string
	LinkSelector         string
	ThumbnailSelector    string
	PublishSelector      string
	PublishFormat        string
	PublishType          string
	InnerPublishSelector string
	PaginationType       string
	PageURLPattern       string
	NextPageSelector     string
	MaxPages             int
	Favicon              string
}
