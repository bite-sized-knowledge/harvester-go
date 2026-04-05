package fetcher

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	utls "github.com/refraction-networking/utls"
)

// newUTLSTransport returns an http.Transport that performs TLS handshakes
// via uTLS using a Chrome ClientHello. This produces a JA3/JA4 fingerprint
// identical to a real Chrome browser, bypassing Cloudflare and similar WAFs
// that fingerprint TLS at the transport layer.
//
// HTTP/2 is intentionally NOT used here — we advertise only "http/1.1" in
// ALPN. Forcing HTTP/1.1 keeps the transport simple: the standard
// http.Transport handles HTTP/1.1 fine over a custom TLS connection via
// DialTLSContext. Supporting h2 via uTLS requires hand-off to
// golang.org/x/net/http2.Transport which adds substantial complexity, and
// HTTP/1.1 is sufficient for every blog harvester use case we have. If we
// ever hit a site that rejects HTTP/1.1 with Chrome fingerprint, we can
// escalate to a fuller library like bogdanfinn/fhttp.
//
// insecure=true disables certificate verification, used as a retry path for
// self-signed / expired certs.
func newUTLSTransport(dialContext func(ctx context.Context, network, addr string) (net.Conn, error), insecure bool) *http.Transport {
	if dialContext == nil {
		dialContext = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext
	}

	return &http.Transport{
		// Custom TLS dial that wraps the plain TCP connection with a uTLS
		// client speaking a Chrome ClientHello — with one critical
		// modification: the ALPN extension advertises only "http/1.1". This
		// keeps us on HTTP/1.1 (which Go's http.Transport can parse over a
		// generic net.Conn) while retaining the JA3/JA4 fingerprint of real
		// Chrome. Supporting h2 via uTLS requires a custom RoundTripper that
		// hands off to golang.org/x/net/http2 after ALPN negotiation —
		// significant complexity for marginal gain, given our harvester is
		// strictly sequential per host.
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			rawConn, err := dialContext(ctx, network, addr)
			if err != nil {
				return nil, fmt.Errorf("dial %s: %w", addr, err)
			}

			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				_ = rawConn.Close()
				return nil, fmt.Errorf("split host:port %s: %w", addr, err)
			}

			uConfig := &utls.Config{
				ServerName:         host,
				InsecureSkipVerify: insecure,
			}
			uConn := utls.UClient(rawConn, uConfig, utls.HelloCustom)

			// Build a Chrome 131 spec and force its ALPN to http/1.1 only.
			// The version is intentionally kept in sync with browser_profile.go's
			// User-Agent strings (Chrome/131) so JA3 fingerprint and UA don't
			// advertise conflicting browser versions — some WAFs cross-check.
			spec, err := utls.UTLSIdToSpec(utls.HelloChrome_131)
			if err != nil {
				_ = rawConn.Close()
				return nil, fmt.Errorf("load chrome spec: %w", err)
			}
			for _, ext := range spec.Extensions {
				if alpn, ok := ext.(*utls.ALPNExtension); ok {
					alpn.AlpnProtocols = []string{"http/1.1"}
				}
			}
			if err := uConn.ApplyPreset(&spec); err != nil {
				_ = rawConn.Close()
				return nil, fmt.Errorf("apply chrome spec: %w", err)
			}

			if err := uConn.HandshakeContext(ctx); err != nil {
				_ = rawConn.Close()
				return nil, fmt.Errorf("uTLS handshake %s: %w", addr, err)
			}
			return uConn, nil
		},

		// Plain TCP dialer for non-TLS hosts (rare in our case — almost all
		// blogs are HTTPS). Required because DialTLSContext only covers
		// https:// URLs; http:// still goes through DialContext.
		DialContext: dialContext,

		ForceAttemptHTTP2:     false,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
	}
}
