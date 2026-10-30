package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNotify_NullObjectPattern(t *testing.T) {
	n := NewDiscord("")
	if _, ok := n.(noopNotifier); !ok {
		t.Errorf("Expected noopNotifier for empty webhook URL, got %T", n)
	}
	// Must not panic
	n.Send(Message{Title: "Test", Body: "Payload", IsError: false})
}

func TestNotify_Discord_SuccessPayload(t *testing.T) {
	var receivedBody string
	var mu sync.Mutex
	done := make(chan struct{})

	n := NewDiscord("https://discord.com/api/webhooks/test/123")
	d, ok := n.(*Discord)
	if !ok {
		t.Fatalf("Expected *Discord, got %T", n)
	}

	d.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		mu.Lock()
		receivedBody = string(body)
		mu.Unlock()
		close(done)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	})

	n.Send(Message{
		Title:   "R2 Sync Complete",
		Body:    "Uploaded vBook/backup.tar.zst",
		IsError: false,
	})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for Discord webhook call")
	}

	mu.Lock()
	defer mu.Unlock()

	var payload map[string]string
	if err := json.Unmarshal([]byte(receivedBody), &payload); err != nil {
		t.Fatalf("Failed to parse JSON body: %v", err)
	}

	expectedPrefix := "[OK] **R2 Sync Complete**\nUploaded vBook/backup.tar.zst"
	if payload["content"] != expectedPrefix {
		t.Errorf("Unexpected Discord content:\nExpected: %q\nReceived: %q", expectedPrefix, payload["content"])
	}
}

func TestNotify_Discord_ErrorPayload(t *testing.T) {
	var receivedBody string
	var mu sync.Mutex
	done := make(chan struct{})

	n := NewDiscord("https://discord.com/api/webhooks/test/123")
	d, ok := n.(*Discord)
	if !ok {
		t.Fatalf("Expected *Discord, got %T", n)
	}

	d.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		mu.Lock()
		receivedBody = string(body)
		mu.Unlock()
		close(done)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	})

	n.Send(Message{
		Title:   "R2 Sync Failed",
		Body:    "Key: vBook/backup.tar.zst\nError: timeout",
		IsError: true,
	})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for Discord webhook call")
	}

	mu.Lock()
	defer mu.Unlock()

	var payload map[string]string
	if err := json.Unmarshal([]byte(receivedBody), &payload); err != nil {
		t.Fatalf("Failed to parse JSON body: %v", err)
	}

	expectedPrefix := "[Error] **R2 Sync Failed**\nKey: vBook/backup.tar.zst\nError: timeout"
	if payload["content"] != expectedPrefix {
		t.Errorf("Unexpected Discord content:\nExpected: %q\nReceived: %q", expectedPrefix, payload["content"])
	}
}

func TestNotify_Discord_StatusWarning(t *testing.T) {
	done := make(chan struct{})
	n := NewDiscord("https://discord.com/api/webhooks/test/123")
	d := n.(*Discord)

	d.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		close(done)
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader("bad request")),
			Header:     make(http.Header),
		}, nil
	})

	n.Send(Message{Title: "Warn", Body: "Testing 400", IsError: true})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for Discord webhook call")
	}
}
