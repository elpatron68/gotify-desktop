package gotifyproxy

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const UIPrefix = "/g"

// sessionCookieMaxAge matches Gotify v3 auth.CookieMaxAge (7 days).
const sessionCookieMaxAge = 7 * 24 * 60 * 60

// Proxy reverse-proxies Gotify through the local Wails asset server and a
// dedicated loopback listener. The loopback UI is preferred: Gotify v3 derives
// its API base from location.pathname, so serving at / avoids /g/ prefix issues.
type Proxy struct {
	mu     sync.RWMutex
	target *url.URL
	token  string

	localURL string
	localLn  net.Listener
}

// Update sets the upstream Gotify base URL and client token.
func (p *Proxy) Update(serverURL, token string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if serverURL == "" || token == "" {
		p.target = nil
		p.token = ""
		return nil
	}
	u, err := url.Parse(serverURL)
	if err != nil {
		return err
	}
	p.target = u
	p.token = token
	return nil
}

// StartLocal binds a loopback HTTP server that serves Gotify at /.
// Safe to call once; subsequent calls return the existing URL.
func (p *Proxy) StartLocal() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.localURL != "" {
		return p.localURL, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen loopback gotify ui: %w", err)
	}
	p.localLn = ln
	p.localURL = "http://" + ln.Addr().String()
	go func() {
		_ = http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p.serve(w, r, false)
		}))
	}()
	return p.localURL, nil
}

// LocalURL returns the loopback UI base URL (no trailing path), or empty if not started.
func (p *Proxy) LocalURL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.localURL
}

// Close stops the loopback listener.
func (p *Proxy) Close() error {
	p.mu.Lock()
	ln := p.localLn
	p.localLn = nil
	p.localURL = ""
	p.mu.Unlock()
	if ln != nil {
		return ln.Close()
	}
	return nil
}

// Middleware returns a Wails AssetServer middleware (legacy /g/ embed path).
func (p *Proxy) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !p.shouldProxy(r) {
			next.ServeHTTP(w, r)
			return
		}
		p.serve(w, r, true)
	})
}

func (p *Proxy) shouldProxy(r *http.Request) bool {
	p.mu.RLock()
	ready := p.target != nil && p.token != ""
	p.mu.RUnlock()
	if !ready {
		return false
	}
	path := r.URL.Path
	if path == UIPrefix || strings.HasPrefix(path, UIPrefix+"/") {
		return true
	}
	return isGotifyAPIPath(path)
}

func isGotifyAPIPath(path string) bool {
	prefixes := []string{
		"/current/",
		"/message",
		"/application",
		"/client",
		"/plugin",
		"/stream",
		"/auth/",
		"/health",
		"/version",
		"/image/",
		"/static/",
		"/swagger",
		"/docs",
		"/mcp",
	}
	for _, prefix := range prefixes {
		if path == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// serve proxies a request. stripUIPrefix is true for the Wails /g/ middleware.
func (p *Proxy) serve(w http.ResponseWriter, r *http.Request, stripUIPrefix bool) {
	p.mu.RLock()
	if p.target == nil || p.token == "" {
		p.mu.RUnlock()
		http.Error(w, "gotify proxy not configured", http.StatusServiceUnavailable)
		return
	}
	target := *p.target
	token := p.token
	p.mu.RUnlock()

	path := r.URL.Path
	uiHTML := path == "/" || path == UIPrefix || path == UIPrefix+"/"
	if stripUIPrefix {
		if path == UIPrefix {
			path = "/"
			uiHTML = true
		} else if strings.HasPrefix(path, UIPrefix+"/") {
			path = strings.TrimPrefix(path, UIPrefix)
			if path == "" {
				path = "/"
			}
			uiHTML = path == "/"
		}
	}

	proxy := httputil.NewSingleHostReverseProxy(&target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = path
		req.Host = target.Host
		// Force identity encoding. Deleting Accept-Encoding is not enough:
		// net/http.Transport may re-add gzip and leave ModifyResponse with a
		// compressed body (breaking HTML injection).
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("X-Gotify-Key", token)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Cookie", "gotify-client-token="+token)
		if path == "/stream" || strings.HasPrefix(path, "/stream/") {
			q := req.URL.Query()
			if q.Get("token") == "" {
				q.Set("token", token)
				req.URL.RawQuery = q.Encode()
			}
		}
		req.Header.Del("Origin")
		req.Header.Del("Referer")
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("X-Frame-Options")
		resp.Header.Del("Content-Security-Policy")
		resp.Header.Del("Set-Cookie")
		setSessionCookie(resp.Header, token)

		if loc := resp.Header.Get("Location"); loc != "" {
			if u, err := url.Parse(loc); err == nil {
				if u.Host == "" || strings.EqualFold(u.Host, target.Host) {
					u.Scheme = ""
					u.Host = ""
					if stripUIPrefix {
						locPath := u.Path
						if locPath == "" || locPath == "/" {
							resp.Header.Set("Location", UIPrefix+"/")
						} else if isGotifyAPIPath(locPath) {
							resp.Header.Set("Location", u.String())
						} else {
							u.Path = UIPrefix + locPath
							resp.Header.Set("Location", u.String())
						}
					}
				}
			}
		}

		if uiHTML && strings.Contains(resp.Header.Get("Content-Type"), "text/html") && resp.Body != nil {
			body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			if err != nil {
				return err
			}
			body = injectSessionBootstrap(body, token)
			resp.Header.Del("Content-Encoding")
			resp.Body = io.NopCloser(bytes.NewReader(body))
			resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
			resp.ContentLength = int64(len(body))
		}
		return nil
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		http.Error(rw, "gotify proxy error: "+err.Error(), http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}

func setSessionCookie(h http.Header, token string) {
	cookie := &http.Cookie{
		Name:     "gotify-client-token",
		Value:    token,
		Path:     "/",
		MaxAge:   sessionCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(sessionCookieMaxAge * time.Second),
	}
	h.Add("Set-Cookie", cookie.String())
}

// injectSessionBootstrap seeds a cookie fallback. Gotify v3 no longer uses
// localStorage for sessions (HttpOnly cookie only).
func injectSessionBootstrap(body []byte, token string) []byte {
	inject := []byte(`<script>(function(){var t=` + strconv.Quote(token) + `;try{document.cookie='gotify-client-token='+encodeURIComponent(t)+'; path=/; max-age=604800; SameSite=Strict';}catch(e){}})();</script>`)
	lower := bytes.ToLower(body)
	if idx := bytes.Index(lower, []byte("<head")); idx >= 0 {
		if end := bytes.IndexByte(body[idx:], '>'); end >= 0 {
			at := idx + end + 1
			out := make([]byte, 0, len(body)+len(inject))
			out = append(out, body[:at]...)
			out = append(out, inject...)
			out = append(out, body[at:]...)
			return out
		}
	}
	return append(inject, body...)
}
