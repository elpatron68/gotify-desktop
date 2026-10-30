package gotifyproxy

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elpatron68/gotify-desktop/internal/applog"
)

const UIPrefix = "/g"

// sessionCookieMaxAge matches Gotify v3 auth.CookieMaxAge (7 days).
const sessionCookieMaxAge = 7 * 24 * 60 * 60

// Proxy reverse-proxies Gotify HTTP through the Wails asset server under /g/.
//
// Important: Wails' AssetServer returns 501 for WebSocket upgrades, so the
// Gotify UI must not open ws://<wails>/g/stream. We inject a bootstrap that
// rewrites WebSocket connections to a local ws://127.0.0.1 bridge (see
// StartWSProxy), which dials the real upstream wss://host/stream?token=.
type Proxy struct {
	mu     sync.RWMutex
	target *url.URL
	token  string

	wsURL string
	wsLn  net.Listener
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
		applog.Printf("proxy %s %s", r.Method, r.URL.Path)
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
	if p.target == nil || p.token == "" {
		p.mu.RUnlock()
		http.Error(w, "gotify proxy not configured", http.StatusServiceUnavailable)
		return
	}
	target := *p.target
	token := p.token
	p.mu.RUnlock()

	path := r.URL.Path
	uiHTML := false
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

	// Never let the embedded UI delete our desktop client token via logout.
	// Gotify's tryAuthenticate() calls logout() on any 4xx; without this the
	// desktop stream token would be destroyed and force a full re-login.
	if r.Method == http.MethodPost && (path == "/auth/logout" || strings.HasPrefix(path, "/auth/logout")) {
		w.Header().Set("Content-Type", "application/json")
		setSessionCookie(w.Header(), token)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(&target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = path
		req.Host = target.Host
		// Force identity encoding so ModifyResponse can rewrite HTML.
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("X-Gotify-Key", token)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Cookie", "gotify-client-token="+token)
		req.Header.Del("Origin")
		req.Header.Del("Referer")
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		applog.Printf("proxy upstream %s %s → %d", r.Method, path, resp.StatusCode)
		resp.Header.Del("X-Frame-Options")
		resp.Header.Del("Content-Security-Policy")
		resp.Header.Del("Set-Cookie")
		setSessionCookie(resp.Header, token)

		if loc := resp.Header.Get("Location"); loc != "" {
			if u, err := url.Parse(loc); err == nil {
				if u.Host == "" || strings.EqualFold(u.Host, target.Host) {
					u.Scheme = ""
					u.Host = ""
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

		if uiHTML && strings.Contains(resp.Header.Get("Content-Type"), "text/html") && resp.Body != nil {
			body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
			if err != nil {
				return err
			}
			// Prefer local WS bridge (same-machine ws://). Direct wss:// to the
			// public Gotify host is often blocked from the Wails WebView origin.
			wsBase := p.WSURL()
			if wsBase == "" {
				wsBase = websocketBase(&target)
			} else {
				wsBase = strings.TrimRight(wsBase, "/") + "/"
			}
			body = injectSessionBootstrap(body, token, wsBase)
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

func websocketBase(target *url.URL) string {
	u := *target
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func setSessionCookie(h http.Header, token string) {
	cookie := &http.Cookie{
		Name:     "gotify-client-token",
		Value:    token,
		Path:     "/",
		MaxAge:   sessionCookieMaxAge,
		HttpOnly: true,
		// Lax (not Strict): the shell and iframe share the Wails origin; Strict
		// has caused the SPA to miss the session cookie on first paint.
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(sessionCookieMaxAge * time.Second),
	}
	h.Add("Set-Cookie", cookie.String())
}

// injectSessionBootstrap installs cookie + token-on-every-request (Android-style)
// + WebSocket rewrite. Wails cannot proxy WebSockets (HTTP 501 on /g/stream).
func injectSessionBootstrap(body []byte, token, wsBase string) []byte {
	script := `<script>(function(){
var t=` + strconv.Quote(token) + `, wsBase=` + strconv.Quote(wsBase) + `;
try{document.cookie='gotify-client-token='+encodeURIComponent(t)+'; path=/; max-age=604800; SameSite=Lax';}catch(e){}
// Gotify ≤2.x reads the client token from localStorage; v3 uses the cookie.
try{localStorage.setItem('gotify-login-key', t);}catch(e){}
function withToken(url){
  try{
    if(typeof url!=='string') return url;
    if(url.indexOf('token=')!==-1) return url;
    if(!/\/(current|message|application|client|plugin|stream|auth|version|health)\b/.test(url) && url.indexOf('/g/')!==0) return url;
    return url+(url.indexOf('?')>=0?'&':'?')+'token='+encodeURIComponent(t);
  }catch(e){return url;}
}
var XO=XMLHttpRequest.prototype.open, XS=XMLHttpRequest.prototype.send;
XMLHttpRequest.prototype.open=function(method,url){
  arguments[1]=withToken(url);
  this.__gdUrl=arguments[1];
  return XO.apply(this,arguments);
};
XMLHttpRequest.prototype.send=function(body){
  try{this.setRequestHeader('X-Gotify-Key',t);}catch(e){}
  try{this.setRequestHeader('Authorization','Bearer '+t);}catch(e){}
  return XS.apply(this,arguments);
};
if(window.fetch){
  var OF=window.fetch;
  window.fetch=function(input,init){
    try{
      if(typeof input==='string'){input=withToken(input);}
      else if(input&&input.url){input=new Request(withToken(input.url),input);}
      init=init||{};
      var h=new Headers(init.headers||(input&&input.headers)||{});
      h.set('X-Gotify-Key',t); h.set('Authorization','Bearer '+t);
      init.headers=h;
    }catch(e){}
    return OF.call(this,input,init);
  };
}
var Orig=window.WebSocket;
function WS(url,protocols){
  try{
    if(typeof url==='string' && /\/stream\/?(\?|$)/.test(url)){
      url=wsBase+'stream?token='+encodeURIComponent(t);
    }
  }catch(e){}
  return protocols===undefined?new Orig(url):new Orig(url,protocols);
}
WS.prototype=Orig.prototype;
WS.CONNECTING=Orig.CONNECTING;WS.OPEN=Orig.OPEN;WS.CLOSING=Orig.CLOSING;WS.CLOSED=Orig.CLOSED;
window.WebSocket=WS;
try{console.info('[gotify-desktop] session bootstrap ok wsBase='+wsBase);}catch(e){}
// Keep the UI off the login route if a desktop token is present.
setInterval(function(){
  try{
    if(location.hash && location.hash.indexOf('login')!==-1){
      location.hash='#/';
    }
  }catch(e){}
}, 1000);
window.addEventListener('message', function(ev){
  try{
    if(ev.data && ev.data.type==='gotify-desktop-refresh'){
      location.hash='#/';
      location.reload();
    }
  }catch(e){}
});
})();</script>`

	inject := []byte(script)
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
