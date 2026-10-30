package gotifyproxy

import "testing"

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
