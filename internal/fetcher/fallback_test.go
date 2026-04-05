package fetcher

import "testing"

func TestLooksHealthy(t *testing.T) {
	cases := []struct {
		name string
		a    Article
		want bool
	}{
		{
			name: "normal article",
			a:    Article{Title: "Deep dive into Kafka", ContentLength: 15000},
			want: true,
		},
		{
			name: "minimum healthy content",
			a:    Article{Title: "OK", ContentLength: fallbackContentMinBytes},
			want: true,
		},
		{
			name: "empty title",
			a:    Article{Title: "", ContentLength: 50000},
			want: false,
		},
		{
			name: "whitespace-only title",
			a:    Article{Title: "   \t\n", ContentLength: 50000},
			want: false,
		},
		{
			name: "dropbox WAF challenge page",
			a:    Article{Title: "", ContentLength: 2007},
			want: false,
		},
		{
			name: "short content with title",
			a:    Article{Title: "A post", ContentLength: 1500},
			want: false,
		},
		{
			name: "just below threshold",
			a:    Article{Title: "A post", ContentLength: fallbackContentMinBytes - 1},
			want: false,
		},
	}

	for _, c := range cases {
		if got := looksHealthy(c.a); got != c.want {
			t.Errorf("%s: looksHealthy(%+v) = %v, want %v", c.name, c.a, got, c.want)
		}
	}
}
