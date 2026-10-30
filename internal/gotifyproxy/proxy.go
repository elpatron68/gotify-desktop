package gotifyproxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
)

const UIPrefix = "/g"

// Proxy reverse-proxies Gotify through the local Wails asset server so the
// embedded UI shares origin with the desktop shell and can stay authenticated
// via the client token (injected as X-Gotify-Key / Cookie).
type Proxy struct {
	mu     sync.RWMutex
	target *url.URL
	token  string
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

// Middleware returns a Wails AssetServer middleware.
func (p *Proxy) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !p.shouldProxy(r) {
			next.ServeHTTP(w, r)
			return
		}
		p.serve(w, r)
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

func (p *Proxy) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	target := *p.target
	token := p.token
	p.mu.RUnlock()

	path := r.URL.Path
	if path == UIPrefix {
		path = "/"
	} else if strings.HasPrefix(path, UIPrefix+"/") {
		path = strings.TrimPrefix(path, UIPrefix)
		if path == "" {
			path = "/"
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
		req.Header.Set("X-Gotify-Key", token)
		req.Header.Set("Authorization", "Bearer "+token)
		// Prefer our session cookie for Gotify v3 UI auth.
		req.Header.Set("Cookie", "gotify-client-token="+token)
		req.Header.Del("Origin")
		req.Header.Del("Referer")
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Allow embedding / same-origin shell usage.
		resp.Header.Del("X-Frame-Options")
		resp.Header.Del("Content-Security-Policy")
		// Keep redirects inside the local proxy when possible.
		if loc := resp.Header.Get("Location"); loc != "" {
			if u, err := url.Parse(loc); err == nil {
				if u.Host == "" || strings.EqualFold(u.Host, target.Host) {
					u.Scheme = ""
					u.Host = ""
					path := u.Path
					if path == "" || path == "/" {
						resp.Header.Set("Location", UIPrefix+"/")
					} else if isGotifyAPIPath(path) {
						resp.Header.Set("Location", u.String())
					} else {
						u.Path = UIPrefix + path
						resp.Header.Set("Location", u.String())
					}
				}
			}
		}
		return nil
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		http.Error(rw, "gotify proxy error: "+err.Error(), http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}
