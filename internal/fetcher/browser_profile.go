package fetcher

// browserProfile bundles all the browser-identifying headers that must stay
// consistent together. Cloudflare and other WAFs cross-check User-Agent
// against sec-ch-ua-* hints and flag mismatches, so we rotate whole profiles
// rather than mixing and matching.
//
// All profiles are Chromium-family (Chrome or Edge) because:
//  1. sec-ch-ua-* headers are Chromium-specific; Firefox/Safari don't send them,
//     so mixing breaks consistency.
//  2. Chromium is ~75% of browser traffic — most natural-looking.
//  3. uTLS also pairs best with Chrome TLS fingerprints.
type browserProfile struct {
	userAgent       string
	secChUa         string
	secChUaMobile   string
	secChUaPlatform string
}

// browserProfiles is the rotation pool. Each entry is internally consistent
// (UA version matches sec-ch-ua version, platform matches).
var browserProfiles = []browserProfile{
	{
		userAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		secChUa:         `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
		secChUaMobile:   "?0",
		secChUaPlatform: `"Windows"`,
	},
	{
		userAgent:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		secChUa:         `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
		secChUaMobile:   "?0",
		secChUaPlatform: `"macOS"`,
	},
	{
		userAgent:       "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		secChUa:         `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
		secChUaMobile:   "?0",
		secChUaPlatform: `"Linux"`,
	},
	{
		userAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		secChUa:         `"Google Chrome";v="130", "Chromium";v="130", "Not_A Brand";v="24"`,
		secChUaMobile:   "?0",
		secChUaPlatform: `"Windows"`,
	},
	{
		userAgent:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		secChUa:         `"Google Chrome";v="130", "Chromium";v="130", "Not_A Brand";v="24"`,
		secChUaMobile:   "?0",
		secChUaPlatform: `"macOS"`,
	},
}
