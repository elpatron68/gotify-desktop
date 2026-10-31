package gotifyproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsGotifyAPIPath(t *testing.T) {
	yes := []string{"/current/user", "/message", "/application", "/client", "/stream", "/auth/local/login", "/static/js/app.js", "/health"}
	for _, p := range yes {
		if !isGotifyAPIPath(p) {
			t.Fatalf("expected proxy path %q", p)
		}
	}
	no := []string{"/", "/assets/index.js", "/wails/ipc", "/g/", "/favicon.ico"}
	for _, p := range no {
		if isGotifyAPIPath(p) {
			t.Fatalf("did not expect proxy path %q", p)
		}
	}
}

func TestInjectSessionBootstrapRewritesWebSocket(t *testing.T) {
	html := []byte("<!doctype html><html><head><title>Gotify</title></head><body></body></html>")
	out := injectSessionBootstrap(html, "gtfyc.test", "wss://gotify.example/")
	s := string(out)
	if !strings.Contains(s, "window.WebSocket") {
		t.Fatal("expected WebSocket rewrite")
	}
	if !strings.Contains(s, "wss://gotify.example/") {
		t.Fatal("expected upstream ws base")
	}
	if !strings.Contains(s, "stream?token=") {
		t.Fatal("expected tokenized stream URL")
	}
	if !strings.Contains(s, "X-Gotify-Key") {
		t.Fatal("expected XHR/fetch token header injection")
	}
	if !strings.Contains(s, "gtfyc.test") {
		t.Fatal("expected token")
	}
	if !strings.Contains(s, "gotify-login-key") {
		t.Fatal("expected localStorage token key for Gotify ≤2.x")
	}
	if !strings.Contains(s, "window.Notification") {
		t.Fatal("expected Notification stub to avoid duplicate browser toasts")
	}
	if !strings.Contains(s, "gotify-view") || !strings.Contains(s, "gv.hidden") {
		t.Fatal("expected login-hash redirect to skip while shell hides Gotify view")
	}
}

func TestStartWSProxy(t *testing.T) {
	p := &Proxy{}
	url, err := p.StartWSProxy()
	if err != nil {
		t.Fatal(err)
	}
	defer p.CloseWS()
	if !strings.HasPrefix(url, "ws://127.0.0.1:") {
		t.Fatalf("wsURL = %q", url)
	}
	if got := p.WSURL(); got != url {
		t.Fatalf("WSURL = %q, want %q", got, url)
	}
	// Second start is idempotent.
	url2, err := p.StartWSProxy()
	if err != nil {
		t.Fatal(err)
	}
	if url2 != url {
		t.Fatalf("second StartWSProxy = %q, want %q", url2, url)
	}
}

func TestProxyInjectsAuthAndSessionCookie(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/current/user" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Gotify-Key"); got != "tok-123" {
			t.Fatalf("X-Gotify-Key = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"name":"admin"}`))
	}))
	defer upstream.Close()

	p := &Proxy{}
	if err := p.Update(upstream.URL, "tok-123"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://wails.local/g/current/user", nil)
	rr := httptest.NewRecorder()
	p.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not fall through")
	})).ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	cookie := rr.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "gotify-client-token=tok-123") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
}

func TestProxyHTMLBootstrap(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept-Encoding"); got != "identity" {
			t.Fatalf("Accept-Encoding = %q, want identity", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html><head></head><body>ok</body></html>")
	}))
	defer upstream.Close()

	p := &Proxy{}
	_ = p.Update(upstream.URL, "tok-xyz")

	req := httptest.NewRequest(http.MethodGet, "http://wails.local/g/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	p.Middleware(http.NotFoundHandler()).ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "tok-xyz") {
		t.Fatalf("missing token bootstrap: %s", body)
	}
	if !strings.Contains(body, "window.WebSocket") {
		t.Fatalf("missing websocket rewrite: %s", body)
	}
	if !strings.Contains(body, "wss://") && !strings.Contains(body, "ws://") {
		t.Fatalf("missing ws base: %s", body)
	}
}

func TestLogoutDoesNotHitUpstream(t *testing.T) {
	hit := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		t.Fatalf("upstream should not be called for logout, path=%s", r.URL.Path)
	}))
	defer upstream.Close()

	p := &Proxy{}
	_ = p.Update(upstream.URL, "tok-keep")

	req := httptest.NewRequest(http.MethodPost, "http://wails.local/g/auth/logout", nil)
	rr := httptest.NewRecorder()
	p.Middleware(http.NotFoundHandler()).ServeHTTP(rr, req)

	if hit {
		t.Fatal("upstream was hit")
	}
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Set-Cookie"), "tok-keep") {
		t.Fatalf("cookie = %q", rr.Header().Get("Set-Cookie"))
	}
}
