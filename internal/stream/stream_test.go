package stream

import (
	"encoding/json"
	"testing"
)

func TestMessageJSON(t *testing.T) {
	raw := `{"id":9,"appid":2,"message":"hello","title":"Test","priority":5,"date":"2026-01-01T00:00:00Z"}`
	var msg Message
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.ID != 9 || msg.AppID != 2 || msg.Message != "hello" || msg.Title != "Test" || msg.Priority != 5 {
		t.Fatalf("unexpected message: %+v", msg)
	}
}
