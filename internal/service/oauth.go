package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/proxy"
)

// TokenTester validates OAuth tokens by making a lightweight API call.
type TokenTester struct{}

func NewTokenTester() *TokenTester {
	return &TokenTester{}
}

// TestToken validates an OAuth token by sending a minimal messages request.
func (t *TokenTester) TestToken(ctx context.Context, token, proxyURL string) error {
	body := `{"model":"claude-haiku-4-5-20251001","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages?beta=true", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("User-Agent", "claude-cli/2.1.89 (external, cli)")
	req.Header.Set("x-app", "cli")

	client := httpClient(proxyURL)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("token invalid: status %d", resp.StatusCode)
	}
	return nil
}

func httpClient(proxyURL string) *http.Client {
	if proxyURL == "" {
		return http.DefaultClient
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return http.DefaultClient
	}

	switch parsed.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsed)
	case "socks5":
		dialer, err := proxy.FromURL(parsed, proxy.Direct)
		if err == nil {
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		}
	}

	return &http.Client{Transport: transport}
}
