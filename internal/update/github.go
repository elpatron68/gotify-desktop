package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	defaultAPIURL     = "https://api.github.com/repos/elpatron68/gotify-desktop/releases/latest"
	apiTimeout        = 20 * time.Second
	downloadTimeout   = 5 * time.Minute
	checksumAssetName = "SHA256SUMS"
	apiMaxAttempts    = 3
)

// Asset is one file attached to a GitHub release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Release is the subset of the GitHub Releases API used for updates.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Client talks to the GitHub Releases API and downloads assets.
type Client struct {
	APIURL    string
	UserAgent string
	HTTP      *http.Client
}

func (c *Client) apiURL() string {
	if c.APIURL != "" {
		return c.APIURL
	}
	return defaultAPIURL
}

func (c *Client) userAgent() string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return "gotify-desktop"
}

func (c *Client) httpClient(timeout time.Duration) *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: githubTransport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("refusing non-HTTPS redirect to %s", req.URL)
			}
			return nil
		},
	}
}

// githubTransport prefers IPv4. Broken IPv6 paths to api.github.com are a
// common cause of multi-second gateway timeouts on Windows.
func githubTransport() http.RoundTripper {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if conn, err := dialer.DialContext(ctx, "tcp4", addr); err == nil {
				return conn, nil
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}
}

// LatestRelease fetches the latest non-prerelease GitHub release.
// Transient gateway failures (502/503/504) are retried a few times.
func (c *Client) LatestRelease(ctx context.Context) (Release, error) {
	var lastErr error
	for attempt := 1; attempt <= apiMaxAttempts; attempt++ {
		rel, err := c.latestReleaseOnce(ctx)
		if err == nil {
			return rel, nil
		}
		lastErr = err
		if attempt == apiMaxAttempts || !isTransientUpdateError(err) {
			break
		}
		delay := time.Duration(attempt) * 500 * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Release{}, ctx.Err()
		case <-timer.C:
		}
	}
	return Release{}, lastErr
}

func isTransientUpdateError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP 502") ||
		strings.Contains(msg, "HTTP 503") ||
		strings.Contains(msg, "HTTP 504") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "Temporary failure") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "TLS handshake timeout")
}

func (c *Client) latestReleaseOnce(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL(), nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", c.userAgent())
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient(apiTimeout).Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Release{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, httpStatusError("github releases", resp.StatusCode, body)
	}
	var rel Release
	if err := json.Unmarshal(body, &rel); err != nil {
		return Release{}, fmt.Errorf("parse release: %w", err)
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return Release{}, fmt.Errorf("release has empty tag_name")
	}
	return rel, nil
}

// FindAsset returns the asset with the given name, or an error if missing.
func (r Release) FindAsset(name string) (Asset, error) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("release asset %q not found", name)
}

// ChecksumAsset returns the SHA256SUMS asset.
func (r Release) ChecksumAsset() (Asset, error) {
	return r.FindAsset(checksumAssetName)
}

// Download writes the asset body to w. Only HTTPS URLs are accepted.
func (c *Client) Download(ctx context.Context, url string, w io.Writer) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("refusing non-HTTPS download URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent())
	resp, err := c.httpClient(downloadTimeout).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return httpStatusError("download", resp.StatusCode, body)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

const maxHTTPErrorDetail = 160

// httpStatusError builds a short, dialog-safe error for non-OK HTTP responses.
// HTML bodies (GitHub web error pages) are omitted; JSON {"message"} is preferred.
func httpStatusError(prefix string, status int, body []byte) error {
	detail := httpErrorDetail(body)
	if detail == "" {
		return fmt.Errorf("%s: HTTP %d", prefix, status)
	}
	return fmt.Errorf("%s: HTTP %d: %s", prefix, status, detail)
}

func httpErrorDetail(body []byte) string {
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return ""
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "<!doctype") || strings.Contains(lower, "<html") {
		return ""
	}
	var gh struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &gh) == nil {
		if m := strings.TrimSpace(gh.Message); m != "" {
			msg = m
		}
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > maxHTTPErrorDetail {
		return msg[:maxHTTPErrorDetail] + "…"
	}
	return msg
}
