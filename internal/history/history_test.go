package history

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendTrimsToMax(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "history.json"))
	for i := 0; i < MaxEntries+5; i++ {
		e := Entry{ID: fmt.Sprintf("%d", i), Title: "t", Body: "b", ReceivedAt: time.Now(), Notified: true}
		if err := s.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	all := s.List(false)
	if len(all) != MaxEntries {
		t.Fatalf("len=%d", len(all))
	}
}

func TestListToastsOnly(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "h.json"))
	_ = s.Append(Entry{ID: "1", Title: "a", Notified: true, ReceivedAt: time.Now()})
	_ = s.Append(Entry{ID: "2", Title: "b", Notified: false, ReceivedAt: time.Now()})
	// Newest first → [2, 1]; toastsOnly keeps notified=true → [1]
	got := s.List(true)
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("%+v", got)
	}
	if len(s.List(false)) != 2 {
		t.Fatalf("%+v", s.List(false))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s := New(path)
	ts := time.Date(2026, 10, 31, 10, 48, 0, 0, time.Local)
	_ = s.Append(Entry{ID: "9", Title: "Hi", Body: "Body", Priority: 5, AppID: 1, ReceivedAt: ts, Notified: true})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2 := New(path)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	got := s2.List(false)
	if len(got) != 1 || got[0].Title != "Hi" || !got[0].Notified {
		t.Fatalf("%+v", got)
	}
}

func TestLoadMissingOrCorrupt(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "missing.json"))
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if len(s.List(false)) != 0 {
		t.Fatal("expected empty")
	}
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	s2 := New(path)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if len(s2.List(false)) != 0 {
		t.Fatal("corrupt should yield empty")
	}
}

func TestFormatMarkdown(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	ts := time.Date(2026, 10, 31, 10, 48, 0, 0, loc)
	got := FormatMarkdown(Entry{Title: "Alert", Body: "Disk full", ReceivedAt: ts}, loc)
	want := "**Alert**\nDisk full\n\n_31.10.2026 10:48_"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got2 := FormatMarkdown(Entry{Body: "only body", ReceivedAt: ts}, loc)
	want2 := "only body\n\n_31.10.2026 10:48_"
	if got2 != want2 {
		t.Fatalf("got %q want %q", got2, want2)
	}
}
