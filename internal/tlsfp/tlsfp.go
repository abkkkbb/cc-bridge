// Package tlsfp provides TLS fingerprint spoofing to mimic Node.js 24.x (Claude CLI).
//
// Go's default crypto/tls produces a distinct JA3/JA4 fingerprint that can be
// detected by Anthropic. This package uses uTLS to replicate the exact TLS
// ClientHello that Node.js 24.x sends, matching the real Claude CLI.
package tlsfp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

// Node.js 24.x cipher suites in wire order (Claude CLI).
var cipherSuites = []uint16{
	// TLS 1.3
	utls.TLS_AES_128_GCM_SHA256,
	utls.TLS_AES_256_GCM_SHA384,
	utls.TLS_CHACHA20_POLY1305_SHA256,
	// TLS 1.2 ECDHE
	utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
	// TLS 1.2 ECDHE CBC
	utls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	utls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	utls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	utls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	// RSA fallback
	utls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	utls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	utls.TLS_RSA_WITH_AES_128_CBC_SHA,
	utls.TLS_RSA_WITH_AES_256_CBC_SHA,
}

var curves = []utls.CurveID{
	utls.X25519,
	utls.CurveP256,
	utls.CurveP384,
}

var sigAlgs = []utls.SignatureScheme{
	utls.ECDSAWithP256AndSHA256,
	utls.ECDSAWithP384AndSHA384,
	utls.PSSWithSHA256,
	utls.PSSWithSHA384,
	utls.PSSWithSHA512,
	utls.PKCS1WithSHA256,
	utls.PKCS1WithSHA384,
	utls.PKCS1WithSHA512,
	utls.PKCS1WithSHA1,
}

// buildSpec creates a utls ClientHelloSpec that matches Node.js 24.x.
func buildSpec() *utls.ClientHelloSpec {
	return &utls.ClientHelloSpec{
		CipherSuites: cipherSuites,
		Extensions: []utls.TLSExtension{
			&utls.SNIExtension{},
			&utls.ExtendedMasterSecretExtension{},
			&utls.RenegotiationInfoExtension{Renegotiation: utls.RenegotiateOnceAsClient},
			&utls.SupportedCurvesExtension{Curves: curves},
			&utls.SupportedPointsExtension{SupportedPoints: []byte{0}}, // uncompressed
			&utls.SessionTicketExtension{},
			&utls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
			&utls.StatusRequestExtension{},
			&utls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: sigAlgs},
			&utls.SCTExtension{},
			&utls.KeyShareExtension{KeyShares: []utls.KeyShare{
				{Group: utls.X25519},
			}},
			&utls.PSKKeyExchangeModesExtension{Modes: []uint8{utls.PskModeDHE}},
			&utls.SupportedVersionsExtension{Versions: []uint16{
				utls.VersionTLS13,
				utls.VersionTLS12,
			}},
		},
		TLSVersMin: utls.VersionTLS12,
		TLSVersMax: utls.VersionTLS13,
	}
}

// dialTLS performs a TLS handshake with utls fingerprinting over an existing net.Conn.
func dialTLS(conn net.Conn, serverName string) (net.Conn, error) {
	tlsConn := utls.UClient(conn, &utls.Config{ServerName: serverName}, utls.HelloCustom)
	if err := tlsConn.ApplyPreset(buildSpec()); err != nil {
		conn.Close()
		return nil, fmt.Errorf("utls apply preset: %w", err)
	}
	if err := tlsConn.Handshake(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("utls handshake: %w", err)
	}
	return tlsConn, nil
}

// hostFromAddr extracts hostname from "host:port".
func hostFromAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// NewTransport creates an http.Transport with TLS fingerprinting.
// Supports direct, HTTP proxy, and SOCKS5 proxy connections.
func NewTransport(proxyURL string) *http.Transport {
	t := &http.Transport{
		ForceAttemptHTTP2:   false, // we handle TLS ourselves
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	if proxyURL == "" {
		// Direct connection with utls
		t.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: 30 * time.Second}
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			return dialTLS(conn, hostFromAddr(addr))
		}
		return t
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		// Fallback: no fingerprinting
		t.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		return t
	}

	switch parsed.Scheme {
	case "socks5", "socks5h":
		t.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			socksDialer, err := proxy.FromURL(parsed, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("socks5 dialer: %w", err)
			}
			conn, err := socksDialer.Dial(network, addr)
			if err != nil {
				return nil, err
			}
			return dialTLS(conn, hostFromAddr(addr))
		}

	case "http", "https":
		// HTTP CONNECT tunnel, then utls over the tunnel
		t.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Connect to proxy
			proxyAddr := parsed.Host
			if !hasPort(proxyAddr) {
				if parsed.Scheme == "https" {
					proxyAddr += ":443"
				} else {
					proxyAddr += ":80"
				}
			}
			dialer := &net.Dialer{Timeout: 30 * time.Second}
			proxyConn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
			if err != nil {
				return nil, fmt.Errorf("proxy dial: %w", err)
			}

			// Send CONNECT
			connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
			if parsed.User != nil {
				// Basic auth not implemented for simplicity; add if needed
				connectReq += "\r\n"
			} else {
				connectReq += "\r\n"
			}
			if _, err := proxyConn.Write([]byte(connectReq)); err != nil {
				proxyConn.Close()
				return nil, fmt.Errorf("proxy CONNECT write: %w", err)
			}

			// Read response (simple parse)
			buf := make([]byte, 4096)
			n, err := proxyConn.Read(buf)
			if err != nil {
				proxyConn.Close()
				return nil, fmt.Errorf("proxy CONNECT read: %w", err)
			}
			resp := string(buf[:n])
			if len(resp) < 12 || resp[9] != '2' {
				proxyConn.Close()
				return nil, fmt.Errorf("proxy CONNECT failed: %s", resp[:min(len(resp), 80)])
			}

			return dialTLS(proxyConn, hostFromAddr(addr))
		}

	default:
		t.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	return t
}

func hasPort(host string) bool {
	_, _, err := net.SplitHostPort(host)
	return err == nil
}
