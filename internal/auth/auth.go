package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client talks to the Gotify HTTP API using Basic Auth (setup) or client token.
type Client struct {
	HTTP *http.Client
}

func New() *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 15 * time.Second},
	}
}

type userInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type clientResponse struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

// VerifyAndCreateClient checks credentials and creates a desktop client token.
// The password is only used for this request and is not retained by this package.
func (c *Client) VerifyAndCreateClient(serverURL, username, password string) (token string, err error) {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" || username == "" || password == "" {
		return "", fmt.Errorf("server URL, username and password are required")
	}

	if err := c.verifyUser(serverURL, username, password); err != nil {
		return "", err
	}

	name := clientName()
	token, err = c.createClient(serverURL, username, password, name)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (c *Client) verifyUser(serverURL, username, password string) error {
	req, err := http.NewRequest(http.MethodGet, serverURL+"/current/user", nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach Gotify server: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusOK:
		var u userInfo
		if err := json.Unmarshal(body, &u); err != nil {
			return fmt.Errorf("unexpected user response: %w", err)
		}
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("invalid username or password")
	default:
		return fmt.Errorf("gotify returned %s: %s", resp.Status, truncate(string(body), 200))
	}
}

func (c *Client) createClient(serverURL, username, password, name string) (string, error) {
	payload, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, serverURL+"/client", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("create client failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create client returned %s: %s", resp.Status, truncate(string(body), 200))
	}

	var cr clientResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", fmt.Errorf("parse client response: %w", err)
	}
	if cr.Token == "" {
		return "", fmt.Errorf("gotify did not return a client token")
	}
	return cr.Token, nil
}

func clientName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "desktop"
	}
	return "gotify-desktop-" + host
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
