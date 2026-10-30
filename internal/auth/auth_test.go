package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyAndCreateClient(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/current/user", func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "admin"})
	})
	mux.HandleFunc("/client", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    2,
			"name":  "gotify-desktop-test",
			"token": "client-token-xyz",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	token, err := c.VerifyAndCreateClient(srv.URL, "admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if token != "client-token-xyz" {
		t.Fatalf("token=%q", token)
	}

	_, err = c.VerifyAndCreateClient(srv.URL, "admin", "wrong")
	if err == nil {
		t.Fatal("expected auth error")
	}
}
