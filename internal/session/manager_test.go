package session

import (
	"testing"
	"time"

	"github.com/elpatron68/gotify-desktop/internal/config"
	"github.com/elpatron68/gotify-desktop/internal/stream"
)

func TestStartStopSnapshot(t *testing.T) {
	m := New()
	var msgs int
	m.OnMessage = func(serverID string, account config.ServerAccount, msg stream.Message) {
		msgs++
	}
	statusCh := make(chan struct{}, 8)
	m.OnStatus = func() {
		select {
		case statusCh <- struct{}{}:
		default:
		}
	}

	acc := config.ServerAccount{
		ID:          "s1",
		Label:       "one",
		ServerURL:   "http://127.0.0.1:1", // nothing listening
		ClientToken: "tok",
		Enabled:     true,
	}
	m.Start(acc)
	// Wait briefly for dial failure status
	select {
	case <-statusCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected status callback")
	}

	st := m.StatusFor([]config.ServerAccount{acc})
	if len(st) != 1 || st[0].ServerID != "s1" {
		t.Fatalf("%+v", st)
	}
	if m.AnyConnected() {
		t.Fatal("should not be connected")
	}
	connected, total := m.Counts()
	if total != 1 || connected != 0 {
		t.Fatalf("counts %d/%d", connected, total)
	}

	m.Stop("s1")
	connected, total = m.Counts()
	if total != 0 {
		t.Fatalf("after stop total=%d", total)
	}
}

func TestStartAllSkipsDisabled(t *testing.T) {
	m := New()
	m.StartAll([]config.ServerAccount{
		{ID: "a", ServerURL: "http://127.0.0.1:1", ClientToken: "t", Enabled: false},
		{ID: "b", ServerURL: "http://127.0.0.1:1", ClientToken: "", Enabled: true},
	})
	_, total := m.Counts()
	if total != 0 {
		t.Fatalf("total=%d", total)
	}
}

func TestUpdateAccountDisables(t *testing.T) {
	m := New()
	acc := config.ServerAccount{
		ID: "s1", ServerURL: "http://127.0.0.1:1", ClientToken: "tok", Enabled: true,
	}
	m.Start(acc)
	acc.Enabled = false
	m.UpdateAccount(acc)
	_, total := m.Counts()
	if total != 0 {
		t.Fatalf("total=%d", total)
	}
}
