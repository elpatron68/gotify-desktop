package history

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elpatron68/gotify-desktop/internal/config"
)

const MaxEntries = 100

type Entry struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	Priority   int       `json:"priority"`
	AppID      uint      `json:"app_id"`
	ReceivedAt time.Time `json:"received_at"`
	Notified   bool      `json:"notified"`
}

type fileShape struct {
	Entries []Entry `json:"entries"`
}

type Store struct {
	mu      sync.RWMutex
	path    string
	entries []Entry // newest first
}

func New(path string) *Store {
	return &Store{path: path}
}

func DefaultPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.json"), nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.entries = nil
		return nil
	}
	if err != nil {
		return err
	}
	var f fileShape
	if err := json.Unmarshal(data, &f); err != nil {
		s.entries = nil
		return nil // corrupt → empty
	}
	s.entries = f.Entries
	if len(s.entries) > MaxEntries {
		s.entries = s.entries[:MaxEntries]
	}
	return nil
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

// saveLocked writes the current entries to disk. Caller must hold s.mu.
func (s *Store) saveLocked() error {
	f := fileShape{Entries: append([]Entry(nil), s.entries...)}
	path := s.path
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) Append(e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append([]Entry{e}, s.entries...)
	if len(s.entries) > MaxEntries {
		s.entries = s.entries[:MaxEntries]
	}
	return s.saveLocked()
}

func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
	return s.saveLocked()
}

func (s *Store) List(toastsOnly bool) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		if toastsOnly && !e.Notified {
			continue
		}
		out = append(out, e)
	}
	return out
}

func FormatMarkdown(e Entry, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	ts := e.ReceivedAt.In(loc).Format("02.01.2006 15:04")
	body := e.Body
	title := strings.TrimSpace(e.Title)
	if title == "" {
		return body + "\n\n_" + ts + "_"
	}
	return "**" + title + "**\n" + body + "\n\n_" + ts + "_"
}
