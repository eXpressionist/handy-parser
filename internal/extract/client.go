package extract

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	http       *http.Client
	maxBytes   int64
	allowLocal bool
}

func NewClient(timeout time.Duration, maxBytes int64, allowLocal bool) *Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		MaxIdleConns: 10, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: timeout,
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !allowLocal && blockedIP(ip) {
				continue
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		}
		return nil, fmt.Errorf("target %q resolves only to blocked addresses", host)
	}
	c := &http.Client{Transport: transport, Timeout: timeout}
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		return validateURL(req.URL)
	}
	return &Client{http: c, maxBytes: maxBytes, allowLocal: allowLocal}
}

func (c *Client) Get(ctx context.Context, rawURL string, headers map[string]string) ([]byte, http.Header, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, err
	}
	if err = validateURL(u); err != nil {
		return nil, nil, err
	}
	if !c.allowLocal && strings.EqualFold(u.Hostname(), "localhost") {
		return nil, nil, fmt.Errorf("localhost is not allowed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "HandyParser/0.1 (+https://github.com/eXpressionist/handy-parser)")
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.1")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.Header, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, c.maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, resp.Header, err
	}
	if int64(len(body)) > c.maxBytes {
		return nil, resp.Header, fmt.Errorf("response exceeds %d bytes", c.maxBytes)
	}
	return body, resp.Header, nil
}

func validateURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http and https URLs are allowed")
	}
	if u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("URL must contain a host and no credentials")
	}
	return nil
}

func blockedIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if p := netip.MustParsePrefix("100.64.0.0/10"); p.Contains(ip) {
		return true
	}
	if p := netip.MustParsePrefix("0.0.0.0/8"); p.Contains(ip) {
		return true
	}
	return false
}
