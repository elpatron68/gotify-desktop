package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message is a Gotify stream message.
type Message struct {
	ID       uint   `json:"id"`
	AppID    uint   `json:"appid"`
	Message  string `json:"message"`
	Title    string `json:"title"`
	Priority int    `json:"priority"`
	Date     string `json:"date"`
}

// Handler is called for each received message.
type Handler func(msg Message)

// StatusHandler reports connection state changes.
type StatusHandler func(connected bool, errMsg string)

// Client maintains a reconnecting websocket to Gotify /stream.
type Client struct {
	URL     string
	OnMessage Handler
	OnStatus  StatusHandler

	mu     sync.Mutex
	cancel context.CancelFunc
	conn   *websocket.Conn
}

// Start begins the reconnect loop. Safe to call once; call Stop to end.
func (c *Client) Start() {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.mu.Unlock()
	go c.loop(ctx)
}

// Stop closes the connection and stops reconnecting.
func (c *Client) Stop() {
	c.mu.Lock()
	cancel := c.cancel
	c.cancel = nil
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
	c.emitStatus(false, "")
}

func (c *Client) loop(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		err := c.connectOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		c.emitStatus(false, msg)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (c *Client) connectOnce(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		Proxy:            http.ProxyFromEnvironment,
	}
	conn, resp, err := dialer.DialContext(ctx, c.URL, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("unauthorized: invalid client token")
		}
		return fmt.Errorf("websocket dial: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	c.emitStatus(true, "")
	defer func() {
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.mu.Unlock()
		_ = conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if c.OnMessage != nil {
			c.OnMessage(msg)
		}
	}
}

func (c *Client) emitStatus(connected bool, errMsg string) {
	if c.OnStatus != nil {
		c.OnStatus(connected, errMsg)
	}
}
