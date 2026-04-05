package fetcher

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"
)

const (
	acceptHeaderValue = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	requestTimeout    = 30 * time.Second
	baseDelay         = 1200 * time.Millisecond
	maxDelay          = 4 * time.Second
	maxRetryAttempts  = 3
)

type Client struct {
	normalClient   *http.Client
	insecureClient *http.Client
	profiles       []browserProfile
	index          atomic.Uint64
	logger         *slog.Logger
	hostThrottle   sync.Map
}

type hostState struct {
	mu          sync.Mutex
	lastRequest time.Time
}

func NewClient(proxyRawURL string, logger *slog.Logger) (*Client, error) {
	dialContext, err := resolveDialContext(proxyRawURL)
	if err != nil {
		return nil, err
	}

	normalTransport := newUTLSTransport(dialContext, false)
	insecureTransport := newUTLSTransport(dialContext, true)

	return &Client{
		normalClient:   &http.Client{Timeout: requestTimeout, Transport: normalTransport},
		insecureClient: &http.Client{Timeout: requestTimeout, Transport: insecureTransport},
		profiles:       browserProfiles,
		logger:         logger,
	}, nil
}

// resolveDialContext returns the base TCP dialer respecting an optional
// socks5 proxy. The returned function is wrapped inside newUTLSTransport for
// the TLS layer.
func resolveDialContext(proxyRawURL string) (func(ctx context.Context, network, addr string) (net.Conn, error), error) {
	if strings.TrimSpace(proxyRawURL) == "" {
		return (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext, nil
	}

	parsed, err := url.Parse(proxyRawURL)
	if err != nil {
		return nil, fmt.Errorf("parse PROXY_URL: %w", err)
	}
	if parsed.Scheme != "socks5" {
		return nil, fmt.Errorf("unsupported proxy scheme %q (expected socks5)", parsed.Scheme)
	}

	dialer, err := proxy.SOCKS5("tcp", parsed.Host, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("create socks5 proxy dialer: %w", err)
	}

	if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
		return contextDialer.DialContext, nil
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}, nil
}

func (c *Client) Get(ctx context.Context, targetURL string) ([]byte, error) {
	if err := c.waitTurn(ctx, targetURL); err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		body, err := c.doGet(ctx, targetURL, false)
		if err == nil {
			return body, nil
		}
		lastErr = err

		if isTLSError(err) {
			c.logger.Warn("TLS error, retrying insecure", "url", targetURL, "attempt", attempt, "error", err)
			body, insecureErr := c.doGet(ctx, targetURL, true)
			if insecureErr == nil {
				return body, nil
			}
			lastErr = insecureErr
		}

		if !shouldRetry(lastErr) || attempt == maxRetryAttempts {
			break
		}

		backoff := retryBackoff(attempt)
		c.logger.Warn("retrying request", "url", targetURL, "attempt", attempt+1, "backoff", backoff.String(), "error", lastErr)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return nil, lastErr
}

func (c *Client) doGet(ctx context.Context, targetURL string, insecure bool) ([]byte, error) {
	client := c.normalClient
	if insecure {
		client = c.insecureClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	profile := c.nextProfile()
	applyBrowserHeaders(req, profile)
	if parsed, err := url.Parse(targetURL); err == nil {
		req.Header.Set("Referer", parsed.Scheme+"://"+parsed.Host+"/")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get %s: %w", targetURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body %s: %w", targetURL, err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http get %s failed with status=%d body=%q", targetURL, resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *Client) nextProfile() browserProfile {
	idx := c.index.Add(1)
	return c.profiles[idx%uint64(len(c.profiles))]
}

// applyBrowserHeaders sets the full complement of headers a real Chrome
// browser sends on a top-level navigation. Consistency across User-Agent and
// sec-ch-ua-* is critical — Cloudflare cross-checks these and flags any
// mismatch. The selected profile groups all these fields together so they
// cannot drift out of sync.
//
// Accept-Encoding is intentionally NOT set. Leaving it unset lets Go's
// http.Transport advertise "gzip" automatically and transparently decompress
// the response. Advertising "br, zstd" would require us to decompress those
// formats ourselves, which is not worth the extra dep for a marginal
// fingerprint improvement.
func applyBrowserHeaders(req *http.Request, p browserProfile) {
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Accept", acceptHeaderValue)
	req.Header.Set("Accept-Language", "ko-KR,ko;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	// Chromium-specific client hints. Must be consistent with User-Agent.
	req.Header.Set("sec-ch-ua", p.secChUa)
	req.Header.Set("sec-ch-ua-mobile", p.secChUaMobile)
	req.Header.Set("sec-ch-ua-platform", p.secChUaPlatform)

	// Fetch metadata headers — what Chrome sends on a top-level navigation
	// triggered by a typed URL or bookmark (sec-fetch-site=none).
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "none")
	req.Header.Set("sec-fetch-user", "?1")
	req.Header.Set("upgrade-insecure-requests", "1")
}

func isTLSError(err error) bool {
	var unknownAuthorityErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthorityErr) {
		return true
	}

	var hostErr x509.HostnameError
	if errors.As(err, &hostErr) {
		return true
	}

	var certInvalidErr x509.CertificateInvalidError
	if errors.As(err, &certInvalidErr) {
		return true
	}

	errText := err.Error()
	return strings.Contains(errText, "UNABLE_TO_VERIFY_LEAF_SIGNATURE") ||
		strings.Contains(errText, "UNABLE_TO_VERIFY_FIRST_CERTIFICATE") ||
		strings.Contains(errText, "x509:")
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	errText := err.Error()
	return strings.Contains(errText, "status=403") ||
		strings.Contains(errText, "status=408") ||
		strings.Contains(errText, "status=425") ||
		strings.Contains(errText, "status=429") ||
		strings.Contains(errText, "status=500") ||
		strings.Contains(errText, "status=502") ||
		strings.Contains(errText, "status=503") ||
		strings.Contains(errText, "status=504") ||
		strings.Contains(errText, "timeout") ||
		strings.Contains(errText, "connection reset") ||
		strings.Contains(errText, "EOF")
}

func retryBackoff(attempt int) time.Duration {
	base := time.Duration(attempt) * 1500 * time.Millisecond
	jitter := time.Duration(rand.IntN(700)) * time.Millisecond
	if delay := base + jitter; delay < maxDelay {
		return delay
	}
	return maxDelay + jitter
}

func (c *Client) waitTurn(ctx context.Context, targetURL string) error {
	host := targetURL
	if parsed, err := url.Parse(targetURL); err == nil && parsed.Host != "" {
		host = parsed.Host
	}

	value, _ := c.hostThrottle.LoadOrStore(host, &hostState{})
	state := value.(*hostState)

	state.mu.Lock()
	defer state.mu.Unlock()

	now := time.Now()
	requiredGap := baseDelay + time.Duration(rand.IntN(1200))*time.Millisecond
	if !state.lastRequest.IsZero() {
		nextAllowed := state.lastRequest.Add(requiredGap)
		if nextAllowed.After(now) {
			wait := nextAllowed.Sub(now)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
	}

	state.lastRequest = time.Now()
	return nil
}
