package push

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListApplications(t *testing.T) {
	var gotKey string
	mux := http.NewServeMux()
	mux.HandleFunc("/application", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		gotKey = r.Header.Get("X-Gotify-Key")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":              2,
				"token":           "secret-app-token-should-not-leak",
				"name":            "Zebra",
				"description":     "last",
				"defaultPriority": 5,
			},
			{
				"id":              1,
				"token":           "other-secret",
				"name":            "Alpha",
				"description":     "first",
				"defaultPriority": 0,
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	apps, err := c.ListApplications(srv.URL, "client-token-abc")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "client-token-abc" {
		t.Fatalf("X-Gotify-Key=%q", gotKey)
	}
	if len(apps) != 2 {
		t.Fatalf("len=%d", len(apps))
	}
	if apps[0].Name != "Alpha" || apps[1].Name != "Zebra" {
		t.Fatalf("order: %#v", apps)
	}
	if apps[0].ID != 1 || apps[0].DefaultPriority != 0 {
		t.Fatalf("alpha=%#v", apps[0])
	}
	raw, err := json.Marshal(apps)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "token") {
		t.Fatalf("token leaked in JSON: %s", raw)
	}
}

func TestSendWithClientTokenAndAppID(t *testing.T) {
	var gotKey string
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/application", func(w http.ResponseWriter, r *http.Request) {
		// Gotify 3: list without tokens
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 7, "name": "Demo", "defaultPriority": 2},
		})
	})
	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		gotKey = r.Header.Get("X-Gotify-Key")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Errorf("body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 9})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	if err := c.Send(srv.URL, "client-token-xyz", 7, "Hi", "Hello world", 4); err != nil {
		t.Fatal(err)
	}
	if gotKey != "client-token-xyz" {
		t.Fatalf("X-Gotify-Key=%q (must be client token)", gotKey)
	}
	if gotBody["title"] != "Hi" || gotBody["message"] != "Hello world" {
		t.Fatalf("body=%#v", gotBody)
	}
	prio, ok := gotBody["priority"].(float64)
	if !ok || int(prio) != 4 {
		t.Fatalf("priority=%#v", gotBody["priority"])
	}
	appID, ok := gotBody["appid"].(float64)
	if !ok || int(appID) != 7 {
		t.Fatalf("appid=%#v", gotBody["appid"])
	}
}

func TestSendUsesAppTokenWhenListStillReturnsIt(t *testing.T) {
	var gotKey string
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/application", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 3, "name": "Legacy", "token": "app-token-legacy"},
		})
	})
	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Gotify-Key")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	if err := c.Send(srv.URL, "client-token", 3, "t", "msg", 1); err != nil {
		t.Fatal(err)
	}
	if gotKey != "app-token-legacy" {
		t.Fatalf("X-Gotify-Key=%q (Gotify ≤2 should use app token)", gotKey)
	}
	if _, has := gotBody["appid"]; has {
		t.Fatalf("appid must not be sent with app token auth: %#v", gotBody)
	}
}

func TestSendEmptyMessage(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/application", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{})
	})
	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	err := c.Send(srv.URL, "client-token", 1, "t", "   ", 0)
	if err == nil {
		t.Fatal("expected error for empty message")
	}
	if called {
		t.Fatal("must not call API for empty message")
	}
}

func TestSendUnauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/application", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "name": "Demo"},
		})
	})
	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	err := c.Send(srv.URL, "bad", 1, "t", "msg", 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSendUnknownApplication(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/application", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "name": "Demo"},
		})
	})
	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Bad Request"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	err := c.Send(srv.URL, "client", 99, "t", "msg", 0)
	if err == nil {
		t.Fatal("expected error for unknown app")
	}
}
