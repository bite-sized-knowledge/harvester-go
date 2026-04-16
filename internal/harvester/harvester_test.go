package harvester

import "testing"

func TestNormalizeLink(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"trim and slash", "  https://example.com/post/  ", "https://example.com/post"},
		{"strip medium source", "https://netflixtechblog.com/foo?source=rss----2615bd06b42e---4", "https://netflixtechblog.com/foo"},
		{"strip utm params", "https://example.com/post?utm_source=x&utm_medium=y", "https://example.com/post"},
		{"preserve non-tracking query", "https://example.com/?p=123", "https://example.com/?p=123"},
		{"mixed query: keep p, drop source", "https://example.com/?p=123&source=rss", "https://example.com/?p=123"},
		{"strip fragment", "https://example.com/post#intro", "https://example.com/post"},
		{"unparsable URL falls back to trim/slash", "ht%3A//[bad", "ht%3A//[bad"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeLink(tc.in); got != tc.want {
				t.Errorf("normalizeLink(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
