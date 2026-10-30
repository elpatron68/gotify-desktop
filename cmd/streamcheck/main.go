// Command streamcheck dials Gotify /stream and prints incoming messages.
//
//	GOTIFY_URL=https://gotify.example.com GOTIFY_TOKEN=... go run ./cmd/streamcheck
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/elpatron68/gotify-desktop/internal/config"
	"github.com/elpatron68/gotify-desktop/internal/stream"
	"github.com/gen2brain/beeep"
)

func main() {
	base := os.Getenv("GOTIFY_URL")
	token := os.Getenv("GOTIFY_TOKEN")
	if base == "" || token == "" {
		log.Fatal("set GOTIFY_URL and GOTIFY_TOKEN")
	}
	normalized, err := config.NormalizeServerURL(base)
	if err != nil {
		log.Fatal(err)
	}
	wsURL, err := config.StreamURL(normalized, token)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("dialing", wsURL)

	client := &stream.Client{
		URL: wsURL,
		OnStatus: func(connected bool, errMsg string) {
			fmt.Printf("status connected=%v err=%q\n", connected, errMsg)
		},
		OnMessage: func(msg stream.Message) {
			raw, _ := json.Marshal(msg)
			fmt.Println("message", string(raw))
			title := msg.Title
			if title == "" {
				title = "Gotify"
			}
			if err := beeep.Notify(title, msg.Message, ""); err != nil {
				fmt.Println("notify error:", err)
			} else {
				fmt.Println("notify ok (beeep)")
			}
		},
	}
	client.Start()
	defer client.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	select {
	case <-sig:
	case <-time.After(5 * time.Minute):
	}
}
