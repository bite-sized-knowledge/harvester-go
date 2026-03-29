package fetcher

import (
	"context"
	"crypto/tls"
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
	userAgents     []string
	index          atomic.Uint64
	logger         *slog.Logger
	hostThrottle   sync.Map
}

type hostState struct {
	mu          sync.Mutex
	lastRequest time.Time
}

func NewClient(proxyRawURL string, logger *slog.Logger) (*Client, error) {
	normalTransport, err := buildTransport(proxyRawURL, false)
	if err != nil {
		return nil, err
	}
	insecureTransport, err := buildTransport(proxyRawURL, true)
	if err != nil {
		return nil, err
	}

	return &Client{
		normalClient:   &http.Client{Timeout: requestTimeout, Transport: normalTransport},
		insecureClient: &http.Client{Timeout: requestTimeout, Transport: insecureTransport},
		userAgents: []string{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
			"Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Edg/124.0.2478.67 Safari/537.36",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 13_6_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
			"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:124.0) Gecko/20100101 Firefox/124.0",
		},
		logger: logger,
	}, nil
}

func buildTransport(proxyRawURL string, insecure bool) (*http.Transport, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if strings.TrimSpace(proxyRawURL) == "" {
		return transport, nil
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
		transport.DialContext = contextDialer.DialContext
		return transport, nil
	}

	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}
	return transport, nil
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
	req.Header.Set("Accept", acceptHeaderValue)
	req.Header.Set("Accept-Language", "ko-KR,ko;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("User-Agent", c.nextUserAgent())
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

func (c *Client) nextUserAgent() string {
	idx := c.index.Add(1)
	return c.userAgents[idx%uint64(len(c.userAgents))]
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
