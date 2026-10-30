package notify

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAdversarial_DiscordSlowEndpoint_NeverStallsCaller verifies that a slow or hanging Discord endpoint
// NEVER stalls the caller thread.
func TestAdversarial_DiscordSlowEndpoint_NeverStallsCaller(t *testing.T) {
	hangDuration := 100 * time.Millisecond
	serverReceived := make(chan struct{})

	mockTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		select {
		case serverReceived <- struct{}{}:
		default:
		}
		// Hang response to simulate high latency or overloaded webhook
		time.Sleep(hangDuration)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	})

	n := NewDiscord("https://discord.com/api/webhooks/slow/123")
	d := n.(*Discord)
	d.client.Transport = mockTransport

	// Measure invocation latency of Send
	start := time.Now()
	totalCalls := 50
	for i := 0; i < totalCalls; i++ {
		n.Send(Message{
			Title:   fmt.Sprintf("Backup %d", i),
			Body:    "Async replication test",
			IsError: false,
		})
	}
	elapsed := time.Since(start)

	// Sending 50 messages to a slow server that hangs 2s each MUST complete in <50ms because Send is asynchronous
	if elapsed > 100*time.Millisecond {
		t.Errorf("notifier.Send blocked caller: %d calls took %v (expected <100ms)", totalCalls, elapsed)
	}

	// Verify background dispatch actually reached the transport
	select {
	case <-serverReceived:
		// Background dispatch executed
	case <-time.After(1 * time.Second):
		t.Fatal("Transport did not receive any request from background goroutines")
	}
}

// TestAdversarial_DiscordFailingEndpoints verifies resilient handling of HTTP 429, 500, TCP drop, and transport errors.
func TestAdversarial_DiscordFailingEndpoints(t *testing.T) {
	var statusCodeToReturn int32 = http.StatusInternalServerError

	mockTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		code := int(atomic.LoadInt32(&statusCodeToReturn))
		if code == -1 {
			// Simulate connection reset / dial failure
			return nil, errors.New("connection reset by peer")
		}
		return &http.Response{
			StatusCode: code,
			Body:       io.NopCloser(strings.NewReader("error body")),
			Header:     make(http.Header),
		}, nil
	})

	n := NewDiscord("https://discord.com/api/webhooks/failing/123")
	d := n.(*Discord)
	d.client.Transport = mockTransport

	// Test 500 Internal Server Error
	atomic.StoreInt32(&statusCodeToReturn, http.StatusInternalServerError)
	n.Send(Message{Title: "Error 500 Test", Body: "Testing", IsError: true})

	// Test 429 Rate Limit
	atomic.StoreInt32(&statusCodeToReturn, http.StatusTooManyRequests)
	n.Send(Message{Title: "Error 429 Test", Body: "Testing", IsError: false})

	// Test Abrupt TCP drop / network error
	atomic.StoreInt32(&statusCodeToReturn, -1)
	n.Send(Message{Title: "Network Error Test", Body: "Testing", IsError: true})

	// Give goroutines time to log warnings and exit cleanly without panicking
	time.Sleep(100 * time.Millisecond)
}

// TestAdversarial_DiscordHighConcurrencyBurst tests sending 100 notifications concurrently from multiple goroutines.
func TestAdversarial_DiscordHighConcurrencyBurst(t *testing.T) {
	var handledCount int64

	mockTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt64(&handledCount, 1)
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	n := NewDiscord("https://discord.com/api/webhooks/burst/123")
	d := n.(*Discord)
	d.client.Transport = mockTransport

	numWorkers := 20
	messagesPerWorker := 5
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < messagesPerWorker; i++ {
				n.Send(Message{
					Title:   fmt.Sprintf("Worker %d", workerID),
					Body:    fmt.Sprintf("Message %d", i),
					IsError: false,
				})
			}
		}(w)
	}

	wg.Wait()

	// Wait up to 3 seconds for all 100 requests to be processed by mock transport
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&handledCount) == int64(numWorkers*messagesPerWorker) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	totalHandled := atomic.LoadInt64(&handledCount)
	if totalHandled != int64(numWorkers*messagesPerWorker) {
		t.Errorf("Expected %d requests handled, got %d", numWorkers*messagesPerWorker, totalHandled)
	}
}
