package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// Notifier defines the common interface for notification backends.
type Notifier interface {
	Send(msg Message)
}

// Message defines the payload sent to notification channels.
type Message struct {
	Title   string
	Body    string
	IsError bool
}

// noopNotifier implements the Null Object Pattern when notifications are disabled.
type noopNotifier struct{}

func (noopNotifier) Send(Message) {}

const (
	defaultHTTPTimeout = 10 * time.Second
)

// Discord delivers messages to a Discord channel via webhook.
type Discord struct {
	webhookURL string
	client     *http.Client
}

// NewDiscord constructs a Notifier for Discord webhooks.
// Returns a noopNotifier if webhookURL is empty, guaranteeing nil-safety.
func NewDiscord(webhookURL string) Notifier {
	cleanURL := strings.TrimSpace(webhookURL)
	if cleanURL == "" {
		return noopNotifier{}
	}
	return &Discord{
		webhookURL: cleanURL,
		client:     &http.Client{Timeout: defaultHTTPTimeout},
	}
}

// Send dispatches the message asynchronously in a background goroutine.
func (d *Discord) Send(msg Message) {
	go d.dispatch(msg)
}

// dispatch executes the HTTP POST request to the Discord webhook endpoint.
func (d *Discord) dispatch(msg Message) {
	badge := "[OK]"
	if msg.IsError {
		badge = "[Error]"
	}

	content := fmt.Sprintf("%s **%s**\n%s", badge, msg.Title, msg.Body)
	payload, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		log.Printf("[warn ] Could not encode Discord notification payload: %v", err)
		return
	}

	resp, err := d.client.Post(d.webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[warn ] Could not send Discord notification: %v. Check network connectivity and webhook URL.", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("[warn ] Discord webhook responded with HTTP %d. Check webhook configuration.", resp.StatusCode)
	}
}
