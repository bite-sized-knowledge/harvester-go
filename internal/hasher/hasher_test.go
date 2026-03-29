package hasher

import "testing"

func TestHashToSha1Base62(t *testing.T) {
	got := HashToSha1Base62("https://example.com")
	want := "7cC4KOvUMf1plpRt6zfbkWIltGu"
	if got != want {
		t.Fatalf("HashToSha1Base62() = %q, want %q", got, want)
	}
}
