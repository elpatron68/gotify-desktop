package session

import (
	"sync"

	"github.com/elpatron68/gotify-desktop/internal/config"
	"github.com/elpatron68/gotify-desktop/internal/stream"
)

// RuntimeStatus is the live connection state for one server.
type RuntimeStatus struct {
	ServerID  string
	Connected bool
	Error     string
}

// MessageHandler receives a Gotify stream message tagged with its server id.
type MessageHandler func(serverID string, account config.ServerAccount, msg stream.Message)

// StatusHandler is called whenever any session connection state changes.
type StatusHandler func()

// Manager runs one stream.Client per enabled server account.
type Manager struct {
	mu sync.Mutex

	OnMessage MessageHandler
	OnStatus  StatusHandler

	sessions map[string]*session
}

type session struct {
	account   config.ServerAccount
	client    *stream.Client
	connected bool
	lastErr   string
}

// New returns an empty session manager.
func New() *Manager {
	return &Manager{sessions: make(map[string]*session)}
}

// StartAll starts streams for every enabled account with a token.
func (m *Manager) StartAll(accounts []config.ServerAccount) {
	for _, acc := range accounts {
		if acc.Enabled && acc.ClientToken != "" {
			m.Start(acc)
		}
	}
}

// StopAll stops every running session.
func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Stop(id)
	}
}

// Start (re)starts the stream for the given account.
func (m *Manager) Start(account config.ServerAccount) {
	if account.ID == "" || account.ClientToken == "" || !account.Enabled {
		return
	}
	wsURL, err := config.StreamURL(account.ServerURL, account.ClientToken)
	if err != nil {
		m.mu.Lock()
		s := m.ensureLocked(account)
		s.connected = false
		s.lastErr = err.Error()
		m.mu.Unlock()
		m.emitStatus()
		return
	}

	m.Stop(account.ID)

	client := &stream.Client{URL: wsURL}
	serverID := account.ID
	accCopy := account
	client.OnMessage = func(msg stream.Message) {
		if m.OnMessage != nil {
			m.OnMessage(serverID, accCopy, msg)
		}
	}
	client.OnStatus = func(connected bool, errMsg string) {
		m.mu.Lock()
		s, ok := m.sessions[serverID]
		if !ok {
			m.mu.Unlock()
			return
		}
		s.connected = connected
		if errMsg != "" {
			s.lastErr = errMsg
		} else if connected {
			s.lastErr = ""
		}
		m.mu.Unlock()
		m.emitStatus()
	}

	m.mu.Lock()
	m.sessions[serverID] = &session{
		account: account,
		client:  client,
	}
	m.mu.Unlock()
	client.Start()
}

// Stop stops the stream for serverID.
func (m *Manager) Stop(serverID string) {
	m.mu.Lock()
	s, ok := m.sessions[serverID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.sessions, serverID)
	client := s.client
	m.mu.Unlock()
	if client != nil {
		client.Stop()
	}
	m.emitStatus()
}

// UpdateAccount refreshes the stored account and restarts the stream if enabled.
func (m *Manager) UpdateAccount(account config.ServerAccount) {
	if !account.Enabled || account.ClientToken == "" {
		m.Stop(account.ID)
		return
	}
	m.Start(account)
}

// Snapshot returns runtime status for all known sessions plus accounts that are
// tracked. Prefer passing current config servers via StatusFor.
func (m *Manager) Snapshot() []RuntimeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RuntimeStatus, 0, len(m.sessions))
	for id, s := range m.sessions {
		out = append(out, RuntimeStatus{
			ServerID:  id,
			Connected: s.connected,
			Error:     s.lastErr,
		})
	}
	return out
}

// StatusFor returns runtime status aligned to the given accounts (stable order).
func (m *Manager) StatusFor(accounts []config.ServerAccount) []RuntimeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RuntimeStatus, 0, len(accounts))
	for _, acc := range accounts {
		st := RuntimeStatus{ServerID: acc.ID}
		if s, ok := m.sessions[acc.ID]; ok {
			st.Connected = s.connected
			st.Error = s.lastErr
		}
		out = append(out, st)
	}
	return out
}

// AnyConnected reports whether at least one session is connected.
func (m *Manager) AnyConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.connected {
			return true
		}
	}
	return false
}

// Counts returns (connected, enabledRunning) among current sessions.
func (m *Manager) Counts() (connected, total int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	total = len(m.sessions)
	for _, s := range m.sessions {
		if s.connected {
			connected++
		}
	}
	return connected, total
}

// HasError reports whether any session has a non-empty last error and is down.
func (m *Manager) HasError() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if !s.connected && s.lastErr != "" {
			return true
		}
	}
	return false
}

// LastError returns the first non-empty session error, if any.
func (m *Manager) LastError() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.lastErr != "" {
			return s.lastErr
		}
	}
	return ""
}

func (m *Manager) ensureLocked(account config.ServerAccount) *session {
	if s, ok := m.sessions[account.ID]; ok {
		s.account = account
		return s
	}
	s := &session{account: account}
	m.sessions[account.ID] = s
	return s
}

func (m *Manager) emitStatus() {
	if m.OnStatus != nil {
		m.OnStatus()
	}
}
