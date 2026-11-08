package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLatestReleaseAndDownload(t *testing.T) {
	payload := []byte("package-bytes")
	sum := sha256.Sum256(payload)
	hexSum := hex.EncodeToString(sum[:])
	sumsBody := hexSum + "  gotify-desktop-linux-amd64\n"

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/elpatron68/gotify-desktop/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		rel := Release{
			TagName: "v0.2.0",
			Assets: []Asset{
				{Name: "SHA256SUMS", BrowserDownloadURL: srv.URL + "/SHA256SUMS"},
				{Name: "gotify-desktop-linux-amd64", BrowserDownloadURL: srv.URL + "/pkg"},
			},
		}
		_ = json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sumsBody))
	})
	mux.HandleFunc("/pkg", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	})
	srv = httptest.NewTLSServer(mux)
	defer srv.Close()

	c := &Client{
		APIURL:    srv.URL + "/repos/elpatron68/gotify-desktop/releases/latest",
		UserAgent: "gotify-desktop/test",
		HTTP:      srv.Client(),
	}
	rel, err := c.LatestRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v0.2.0" {
		t.Fatalf("tag=%q", rel.TagName)
	}
	if !IsNewer("0.1.9", rel.TagName) {
		t.Fatal("expected newer")
	}

	dir := t.TempDir()
	res, err := c.DownloadAndVerify(context.Background(), rel, Target{
		Kind:      InstallBinary,
		AssetName: "gotify-desktop-linux-amd64",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(res.PackagePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(payload) {
		t.Fatalf("pkg=%q", data)
	}
	if filepath.Base(res.PackagePath) != "gotify-desktop-linux-amd64" {
		t.Fatalf("path=%q", res.PackagePath)
	}
}

func TestDownloadRejectsHTTP(t *testing.T) {
	c := &Client{}
	err := c.Download(context.Background(), "http://example.com/x", &strings.Builder{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHTTPErrorDetailOmitsHTML(t *testing.T) {
	err := httpStatusError("github releases", 404, []byte("<!DOCTYPE html><html><body>GitHub</body></html>"))
	got := err.Error()
	if got != "github releases: HTTP 404" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "<") {
		t.Fatalf("HTML leaked: %q", got)
	}
}

func TestHTTPErrorDetailUsesGitHubJSONMessage(t *testing.T) {
	err := httpStatusError("github releases", 404, []byte(`{"message":"Not Found","documentation_url":"https://docs.github.com"}`))
	got := err.Error()
	if got != "github releases: HTTP 404: Not Found" {
		t.Fatalf("got %q", got)
	}
}

func TestHTTPErrorDetailTruncates(t *testing.T) {
	long := strings.Repeat("x", maxHTTPErrorDetail+50)
	err := httpStatusError("download", 500, []byte(long))
	got := err.Error()
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected truncation: %q", got)
	}
	if len(got) > len("download: HTTP 500: ")+maxHTTPErrorDetail+3 {
		t.Fatalf("too long: %d", len(got))
	}
}

func TestLatestReleaseRejectsHTMLBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/x/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<!DOCTYPE html><html><head><title>Not Found</title></head></html>"))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	c := &Client{APIURL: srv.URL + "/repos/x/releases/latest", HTTP: srv.Client()}
	_, err := c.LatestRelease(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "<") {
		t.Fatalf("HTML leaked: %v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("got %v", err)
	}
}

func TestLatestReleaseRetriesTransientGateway(t *testing.T) {
	var hits int
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/x/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusGatewayTimeout)
			_, _ = w.Write([]byte("<!DOCTYPE html><html></html>"))
			return
		}
		_ = json.NewEncoder(w).Encode(Release{TagName: "v1.2.3"})
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	c := &Client{APIURL: srv.URL + "/repos/x/releases/latest", HTTP: srv.Client()}
	rel, err := c.LatestRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.2.3" {
		t.Fatalf("tag=%q", rel.TagName)
	}
	if hits != 3 {
		t.Fatalf("hits=%d", hits)
	}
}

func TestLatestReleaseDoesNotRetryNotFound(t *testing.T) {
	var hits int
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/x/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	c := &Client{APIURL: srv.URL + "/repos/x/releases/latest", HTTP: srv.Client()}
	_, err := c.LatestRelease(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if hits != 1 {
		t.Fatalf("hits=%d want 1", hits)
	}
}

func TestDownloadChecksumMismatch(t *testing.T) {
	payload := []byte("package-bytes")
	sumsBody := strings.Repeat("a", 64) + "  gotify-desktop-linux-amd64\n"

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/x/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{
			TagName: "v1.0.0",
			Assets: []Asset{
				{Name: "SHA256SUMS", BrowserDownloadURL: srv.URL + "/SHA256SUMS"},
				{Name: "gotify-desktop-linux-amd64", BrowserDownloadURL: srv.URL + "/pkg"},
			},
		})
	})
	mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sumsBody))
	})
	mux.HandleFunc("/pkg", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	})
	srv = httptest.NewTLSServer(mux)
	defer srv.Close()

	c := &Client{APIURL: srv.URL + "/repos/x/releases/latest", HTTP: srv.Client()}
	rel, err := c.LatestRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.DownloadAndVerify(context.Background(), rel, Target{
		Kind: InstallBinary, AssetName: "gotify-desktop-linux-amd64",
	}, t.TempDir())
	if err == nil {
		t.Fatal("expected checksum error")
	}
}
