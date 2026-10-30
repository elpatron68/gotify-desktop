package gotifyproxy

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/elpatron68/gotify-desktop/internal/applog"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// StartWSProxy listens on 127.0.0.1 for Gotify UI WebSocket connections and
// bridges them to the upstream Gotify /stream. Wails cannot upgrade WebSockets
// on the asset server; a local ws:// bridge keeps the UI live-updating.
func (p *Proxy) StartWSProxy() (wsURL string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.wsURL != "" {
		return p.wsURL, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("ws proxy listen: %w", err)
	}
	p.wsLn = ln
	p.wsURL = "ws://" + ln.Addr().String()
	applog.Printf("ws proxy listening on %s", p.wsURL)

	go http.Serve(ln, http.HandlerFunc(p.handleWSBridge))
	return p.wsURL, nil
}

func (p *Proxy) handleWSBridge(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	target := p.target
	token := p.token
	p.mu.RUnlock()
	if target == nil || token == "" {
		http.Error(w, "not configured", http.StatusServiceUnavailable)
		return
	}

	clientConn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		applog.Printf("ws proxy upgrade failed: %v", err)
		return
	}
	defer clientConn.Close()

	upstream := *target
	switch upstream.Scheme {
	case "https":
		upstream.Scheme = "wss"
	default:
		upstream.Scheme = "ws"
	}
	upstream.Path = strings.TrimRight(upstream.Path, "/") + "/stream"
	q := upstream.Query()
	if t := r.URL.Query().Get("token"); t != "" {
		q.Set("token", t)
	} else {
		q.Set("token", token)
	}
	upstream.RawQuery = q.Encode()

	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	applog.Printf("ws proxy dialing upstream %s", upstream.Host+"/stream")
	upConn, resp, err := dialer.Dial(upstream.String(), nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		applog.Printf("ws proxy upstream dial failed status=%d err=%v", status, err)
		_ = clientConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "upstream unavailable"))
		return
	}
	defer upConn.Close()
	applog.Printf("ws proxy bridged ok")

	errc := make(chan error, 2)
	go copyWS(clientConn, upConn, errc)
	go copyWS(upConn, clientConn, errc)
	err = <-errc
	applog.Printf("ws proxy bridge closed: %v", err)
}

func copyWS(dst, src *websocket.Conn, errc chan<- error) {
	for {
		msgType, data, err := src.ReadMessage()
		if err != nil {
			errc <- err
			return
		}
		if err := dst.WriteMessage(msgType, data); err != nil {
			errc <- err
			return
		}
	}
}

// WSURL returns the local WebSocket bridge base (ws://127.0.0.1:port) or empty.
func (p *Proxy) WSURL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.wsURL
}

// CloseWS stops the WebSocket bridge listener.
func (p *Proxy) CloseWS() {
	p.mu.Lock()
	ln := p.wsLn
	p.wsLn = nil
	p.wsURL = ""
	p.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
}
