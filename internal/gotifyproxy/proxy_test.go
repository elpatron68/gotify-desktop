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

func TestInjectSessionBootstrap(t *testing.T) {
	html := []byte("<!doctype html><html><head><title>Gotify</title></head><body></body></html>")
	out := injectSessionBootstrap(html, "gtfy.c.testtoken")
	s := string(out)
	if !strings.Contains(s, "gtfy.c.testtoken") {
		t.Fatal("expected token in bootstrap")
	}
	headIdx := strings.Index(s, "<head>")
	scriptIdx := strings.Index(s, "gotify-client-token")
	if headIdx < 0 || scriptIdx < headIdx {
		t.Fatalf("bootstrap should be inside/after <head>: %s", s)
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
	if !strings.Contains(strings.ToLower(cookie), "httponly") {
		t.Fatalf("expected HttpOnly cookie, got %q", cookie)
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
}

func TestLocalUIServesAtRoot(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<html><head></head><body>ui</body></html>")
		case "/current/user":
			if r.Header.Get("X-Gotify-Key") != "tok-local" {
				t.Fatalf("missing key")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":1,"name":"admin"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	p := &Proxy{}
	_ = p.Update(upstream.URL, "tok-local")
	base, err := p.StartLocal()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "tok-local") {
		t.Fatalf("html = %s", body)
	}

	resp2, err := http.Get(base + "/current/user")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("status = %d", resp2.StatusCode)
	}
}
