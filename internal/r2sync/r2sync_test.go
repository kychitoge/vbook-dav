package r2sync

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSyncer_NilSafety(t *testing.T) {
	var s *Syncer
	// All operations must gracefully no-op without panics
	if s.Enqueue(Job{}) {
		t.Errorf("Expected Enqueue on nil syncer to return false")
	}
	s.OnDone(func(Job, error) {})
	s.Start(context.Background())
}

func TestSyncer_QueueFullDrop(t *testing.T) {
	s := &Syncer{
		queue: make(chan Job, 2),
	}

	var droppedJob Job
	var droppedErr error
	var mu sync.Mutex

	s.OnDone(func(j Job, err error) {
		mu.Lock()
		droppedJob = j
		droppedErr = err
		mu.Unlock()
	})

	// Fill queue buffer
	if !s.Enqueue(Job{R2Key: "file1.zst"}) {
		t.Errorf("Failed to enqueue job 1")
	}
	if !s.Enqueue(Job{R2Key: "file2.zst"}) {
		t.Errorf("Failed to enqueue job 2")
	}

	// 3rd job must drop non-blockingly
	ok := s.Enqueue(Job{R2Key: "file3.zst"})
	if ok {
		t.Errorf("Expected 3rd enqueue to return false on full buffer")
	}

	mu.Lock()
	defer mu.Unlock()
	if droppedJob.R2Key != "file3.zst" {
		t.Errorf("Expected dropped job file3.zst, got %s", droppedJob.R2Key)
	}
	if droppedErr == nil || !strings.Contains(droppedErr.Error(), "sync queue full") {
		t.Errorf("Expected sync queue full error, got %v", droppedErr)
	}
}

func TestSyncer_EnqueueBlockingPaced(t *testing.T) {
	s := &Syncer{
		queue: make(chan Job, 1),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Fill queue
	if err := s.enqueueBlocking(ctx, Job{R2Key: "item1"}); err != nil {
		t.Fatalf("Failed to enqueue item1: %v", err)
	}

	// Second enqueue in goroutine
	enqueued := make(chan struct{})
	go func() {
		_ = s.enqueueBlocking(ctx, Job{R2Key: "item2"})
		close(enqueued)
	}()

	// Confirm it is blocked
	select {
	case <-enqueued:
		t.Fatal("enqueueBlocking should have blocked on full channel")
	case <-time.After(50 * time.Millisecond):
		// Expected: still blocked
	}

	// Drain item1
	<-s.queue

	// Now it should unblock
	select {
	case <-enqueued:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("enqueueBlocking timed out after channel drained")
	}
}

func TestSyncer_KeyNormalization(t *testing.T) {
	s := &Syncer{
		queue: make(chan Job, 10),
	}

	tests := []struct {
		inputKey string
		expected string
	}{
		{"/vBook/backup.tar.zst", "vBook/backup.tar.zst"},
		{"vBook\\nested\\file.zst", "vBook/nested/file.zst"},
		{"///books/tru_tien.epub", "books/tru_tien.epub"},
		{"clean_file.txt", "clean_file.txt"},
	}

	for _, tt := range tests {
		s.Enqueue(Job{R2Key: tt.inputKey})
		job := <-s.queue
		if job.R2Key != tt.expected {
			t.Errorf("Key normalization mismatch for %q: expected %q, got %q", tt.inputKey, tt.expected, job.R2Key)
		}
	}
}

func TestSyncer_IgnoreHiddenAndTempFiles(t *testing.T) {
	s := &Syncer{
		queue: make(chan Job, 10),
	}

	ignoredKeys := []string{
		".gitkeep",
		"data/.gitkeep",
		".DS_Store",
		"vbook_backup/.DS_Store",
		"upload.tar.zst.tmp",
		"vbook_backup/partial.tmp",
		".hidden_folder/file.zst",
	}

	for _, k := range ignoredKeys {
		enqueued := s.Enqueue(Job{R2Key: k})
		if enqueued {
			t.Errorf("Expected key %q to be ignored and dropped, but was enqueued", k)
		}
	}

	if len(s.queue) != 0 {
		t.Errorf("Queue should be empty, but has %d items", len(s.queue))
	}
}
