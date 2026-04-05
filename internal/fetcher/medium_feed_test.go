package fetcher

import "testing"

func TestIsMediumArticleURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		// Real Medium post URLs — should all pass.
		{"https://netflixtechblog.com/mount-mayhem-at-netflix-f3b09b68beac", true},
		{"https://medium.com/netflix-techblog/scaling-llm-post-training-0046f8790194", true},
		{"https://techblog.gccompany.co.kr/여기어때-이벤트-기반-통합-알림-3d2961d1e849", true},
		{"https://medium.com/daangn/이벤트센터-개발기-e3c240945882", true},
		{"https://publication.medium.com/post-slug-abcdef123456", true},

		// Leaked navigation URLs — should all be rejected.
		// These are the actual rows observed in prod (blog_id=1099 Netflix, 40 rows).
		{"https://netflixtechblog.com/tagged/data-catalog", false},
		{"https://netflixtechblog.com/tagged/message-queue", false},
		{"https://netflixtechblog.com/tagged/rdf", false},
		{"https://netflixtechblog.com/about", false},
		{"https://medium.com/search?q=llm", false},
		{"https://medium.com/m/signin", false},
		{"https://netflixtechblog.com/latest", false},
		{"https://netflixtechblog.com/trending", false},
		{"https://medium.com/_/api/...", false},

		// Edge cases.
		{"", false},
		{"   ", false},
		{"https://netflixtechblog.com/", false},
		{"https://netflixtechblog.com", false},
		{"not a url", true}, // url.Parse is lenient; relative "not a url" returns empty path — treat as "not obviously nav"
	}

	for _, c := range cases {
		got := isMediumArticleURL(c.url)
		if got != c.want {
			t.Errorf("isMediumArticleURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
