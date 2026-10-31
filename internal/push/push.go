package push

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Client talks to the Gotify HTTP API for listing apps and sending messages.
type Client struct {
	HTTP *http.Client
}

// New returns a Client with a default timeout.
func New() *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 15 * time.Second},
	}
}

// Application is a Gotify application without its secret token.
type Application struct {
	ID              uint   `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	DefaultPriority int    `json:"defaultPriority"`
}

type applicationDetail struct {
	ID              uint   `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	DefaultPriority int    `json:"defaultPriority"`
	Token           string `json:"token"`
}

// ListApplications returns applications for the account, sorted by name.
// Tokens from the API response are omitted from the returned values.
func (c *Client) ListApplications(serverURL, clientToken string) ([]Application, error) {
	details, err := c.listApplicationDetails(serverURL, clientToken)
	if err != nil {
		return nil, err
	}
	out := make([]Application, 0, len(details))
	for _, d := range details {
		out = append(out, Application{
			ID:              d.ID,
			Name:            d.Name,
			Description:     d.Description,
			DefaultPriority: d.DefaultPriority,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (c *Client) listApplicationDetails(serverURL, clientToken string) ([]applicationDetail, error) {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	clientToken = strings.TrimSpace(clientToken)
	if serverURL == "" || clientToken == "" {
		return nil, fmt.Errorf("server URL and client token are required")
	}

	req, err := http.NewRequest(http.MethodGet, serverURL+"/application", nil)
	if err != nil {
		return nil, err
	}
	setClientAuth(req, clientToken)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list applications failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list applications returned %s: %s", resp.Status, truncate(string(body), 200))
	}

	var details []applicationDetail
	if err := json.Unmarshal(body, &details); err != nil {
		return nil, fmt.Errorf("parse applications: %w", err)
	}
	return details, nil
}

// Send posts a message as the given application.
//
// Gotify 3+: authenticate with the client token and pass "appid" in the body
// (application tokens are no longer returned by GET endpoints).
// Gotify ≤2: if GET /application still includes a token for that app, use it.
func (c *Client) Send(serverURL, clientToken string, appID uint, title, message string, priority int) error {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	clientToken = strings.TrimSpace(clientToken)
	message = strings.TrimSpace(message)
	if serverURL == "" || clientToken == "" {
		return fmt.Errorf("server URL and client token are required")
	}
	if appID == 0 {
		return fmt.Errorf("application is required")
	}
	if message == "" {
		return fmt.Errorf("message is required")
	}

	details, err := c.listApplicationDetails(serverURL, clientToken)
	if err != nil {
		return err
	}
	var found bool
	var appToken string
	for _, d := range details {
		if d.ID == appID {
			found = true
			appToken = d.Token
			break
		}
	}
	if !found {
		return fmt.Errorf("application not found")
	}

	payload := map[string]any{
		"title":    title,
		"message":  message,
		"priority": priority,
	}
	authToken := clientToken
	if appToken != "" {
		// Gotify ≤2 path: send with the application token; appid is ignored then.
		authToken = appToken
	} else {
		payload["appid"] = appID
	}
	return c.postMessage(serverURL, authToken, payload)
}

func (c *Client) postMessage(serverURL, token string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, serverURL+"/message", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Gotify-Key", token)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("send message failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("unauthorized")
	default:
		return fmt.Errorf("send message returned %s: %s", resp.Status, truncate(string(respBody), 200))
	}
}

func setClientAuth(req *http.Request, clientToken string) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Gotify-Key", clientToken)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
