package fetcher

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	utls "github.com/refraction-networking/utls"
)

// cachedChromeSpec holds the Chrome 131 ClientHelloSpec with ALPN forced to
// http/1.1.  It is built once at package init and cloned for every connection
// via cloneChromeSpec, because ApplyPreset mutates extension objects through
// shared pointers.
var cachedChromeSpec utls.ClientHelloSpec

func init() {
	spec, err := utls.UTLSIdToSpec(utls.HelloChrome_131)
	if err != nil {
		// The spec is compiled-in; a failure here is a programmer error.
		panic(fmt.Sprintf("utls: load chrome 131 spec: %v", err))
	}
	for _, ext := range spec.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			alpn.AlpnProtocols = []string{"http/1.1"}
		}
	}
	cachedChromeSpec = spec
}

// cloneChromeSpec returns a shallow copy of cachedChromeSpec with a fresh
// Extensions slice.  Each element is the same pointer as the cached version
// except for *ALPNExtension, which is deep-copied because ApplyPreset
// mutates extension structs through the shared interface pointers (e.g. it
// overwrites SNIExtension.ServerName).  A fresh ALPNExtension avoids any
// cross-connection mutation of the ALPN value we patched in init().
//
// SNIExtension is intentionally NOT cloned: ApplyPreset unconditionally
// overwrites ServerName with uconn.config.ServerName, so sharing is safe as
// long as connections are not concurrent — which matches the sequential-
// per-host model of the harvester.  If concurrency is ever added, clone SNI
// as well.
func cloneChromeSpec() utls.ClientHelloSpec {
	clone := cachedChromeSpec
	clone.Extensions = make([]utls.TLSExtension, len(cachedChromeSpec.Extensions))
	copy(clone.Extensions, cachedChromeSpec.Extensions)
	for i, ext := range clone.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			alpnCopy := *alpn
			alpnCopy.AlpnProtocols = make([]string, len(alpn.AlpnProtocols))
			copy(alpnCopy.AlpnProtocols, alpn.AlpnProtocols)
			clone.Extensions[i] = &alpnCopy
		}
	}
	clone.CipherSuites = make([]uint16, len(cachedChromeSpec.CipherSuites))
	copy(clone.CipherSuites, cachedChromeSpec.CipherSuites)
	return clone
}

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

			spec := cloneChromeSpec()
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

		DialContext: dialContext,

		ForceAttemptHTTP2:     false,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
	}
}

// newStdTransport returns an http.Transport using Go's standard crypto/tls.
// It supports h2 natively and serves as a fallback when uTLS handshakes are
// rejected by servers that detect the ALPN http/1.1-only anomaly in the
// Chrome fingerprint.
func newStdTransport(dialContext func(ctx context.Context, network, addr string) (net.Conn, error), insecure bool) *http.Transport {
	if dialContext == nil {
		dialContext = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext
	}

	return &http.Transport{
		DialContext: dialContext,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure,
		},
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
	}
}
