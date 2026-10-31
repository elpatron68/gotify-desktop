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
