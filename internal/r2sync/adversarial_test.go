package r2sync

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// TestAdversarial_QueueSaturation_NonBlocking tests that enqueueing 200+ jobs into the 64-capacity queue
// never blocks, drops excess jobs cleanly, and notifies via onDone.
func TestAdversarial_QueueSaturation_NonBlocking(t *testing.T) {
	s := &Syncer{
		queue: make(chan Job, 64),
	}

	var droppedCount int64
	var enqueuedCount int64

	s.OnDone(func(j Job, err error) {
		if err != nil && err.Error() == "sync queue full, job dropped" {
			atomic.AddInt64(&droppedCount, 1)
		}
	})

	startTime := time.Now()
	totalJobs := 200

	for i := 0; i < totalJobs; i++ {
		ok := s.Enqueue(Job{
			Action: ActionUpload,
			R2Key:  fmt.Sprintf("backup_%03d.tar.zst", i),
		})
		if ok {
			enqueuedCount++
		}
	}
	elapsed := time.Since(startTime)

	if elapsed > 100*time.Millisecond {
		t.Errorf("Enqueueing %d jobs took %v; expected non-blocking execution (<100ms)", totalJobs, elapsed)
	}

	if enqueuedCount != 64 {
		t.Errorf("Expected exactly 64 jobs enqueued into buffer, got %d", enqueuedCount)
	}

	expectedDrops := int64(totalJobs - 64)
	actualDrops := atomic.LoadInt64(&droppedCount)
	if actualDrops != expectedDrops {
		t.Errorf("Expected %d dropped jobs, got %d", expectedDrops, actualDrops)
	}

	if len(s.queue) != 64 {
		t.Errorf("Queue length should be 64, got %d", len(s.queue))
	}
}

// TestAdversarial_ConcurrentQueueSaturation tests high-concurrency burst from multiple simulated WebDAV handlers.
func TestAdversarial_ConcurrentQueueSaturation(t *testing.T) {
	s := &Syncer{
		queue: make(chan Job, 64),
	}

	var droppedCount int64
	var acceptedCount int64

	s.OnDone(func(j Job, err error) {
		if err != nil {
			atomic.AddInt64(&droppedCount, 1)
		}
	})

	numGoroutines := 20
	jobsPerGoroutine := 15
	totalJobs := numGoroutines * jobsPerGoroutine // 300 jobs

	var wg sync.WaitGroup
	startSignal := make(chan struct{})

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			<-startSignal
			for i := 0; i < jobsPerGoroutine; i++ {
				ok := s.Enqueue(Job{
					Action: ActionUpload,
					R2Key:  fmt.Sprintf("worker_%d_job_%d.zst", gid, i),
				})
				if ok {
					atomic.AddInt64(&acceptedCount, 1)
				}
			}
		}(g)
	}

	close(startSignal)
	wg.Wait()

	accepted := atomic.LoadInt64(&acceptedCount)
	dropped := atomic.LoadInt64(&droppedCount)

	if accepted != 64 {
		t.Errorf("Expected exactly 64 accepted jobs, got %d", accepted)
	}
	if accepted+dropped != int64(totalJobs) {
		t.Errorf("Total processed jobs mismatch: accepted (%d) + dropped (%d) != %d", accepted, dropped, totalJobs)
	}
}

// TestAdversarial_PacedStartupScan_NoDrops verifies that startupScan paces its enqueueing
// and NEVER drops a single job even when local folder has 150+ items (far exceeding buffer size 64).
func TestAdversarial_PacedStartupScan_NoDrops(t *testing.T) {
	// In-memory S3 mock via roundTripFunc to guarantee hermetic test execution without TCP sockets
	mockTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet && req.URL.Query().Has("list-type") {
			// S3 ListObjectsV2 response with 0 keys
			xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
    <Name>test-bucket</Name>
    <IsTruncated>false</IsTruncated>
</ListBucketResult>`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(xmlBody)),
				Header:     make(http.Header),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	httpClient := &http.Client{Transport: mockTransport}

	// 2. Create temp directory with 150 local files
	tempDir, err := os.MkdirTemp("", "vbook_syncer_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	totalFiles := 150
	for i := 0; i < totalFiles; i++ {
		fname := filepath.Join(tempDir, fmt.Sprintf("file_%03d.txt", i))
		if err := os.WriteFile(fname, []byte("data"), 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}
	}
	// Also add a .tmp file which MUST be skipped
	tmpFile := filepath.Join(tempDir, "upload_in_progress.txt.tmp")
	_ = os.WriteFile(tmpFile, []byte("partial"), 0644)

	// 3. Construct syncer with custom S3 client
	s3Client := s3.New(s3.Options{
		BaseEndpoint: aws.String("https://dummy.r2.cloudflarestorage.com"),
		Region:       "auto",
		Credentials:  credentials.NewStaticCredentialsProvider("key", "secret", ""),
		HTTPClient:   httpClient,
		UsePathStyle: true,
	})

	syncer := &Syncer{
		client:  s3Client,
		bucket:  "test-bucket",
		baseDir: tempDir,
		queue:   make(chan Job, 64),
	}

	var droppedCount int64
	syncer.OnDone(func(j Job, err error) {
		if err != nil {
			atomic.AddInt64(&droppedCount, 1)
		}
	})

	// 4. Run consumer goroutine in background draining the queue to simulate worker processing
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var consumedCount int64
	consumerDone := make(chan struct{})

	go func() {
		defer close(consumerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-syncer.queue:
				atomic.AddInt64(&consumedCount, 1)
				if filepath.Ext(job.R2Key) == ".tmp" {
					t.Errorf("Unexpected .tmp file queued: %s", job.R2Key)
				}
				if atomic.LoadInt64(&consumedCount) == int64(totalFiles) {
					return
				}
			}
		}
	}()

	// 5. Execute startupScan directly
	syncer.startupScan(ctx)

	// Wait for consumer to finish draining all 150 jobs
	select {
	case <-consumerDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("Consumer timed out. Consumed: %d / %d", atomic.LoadInt64(&consumedCount), totalFiles)
	}

	if atomic.LoadInt64(&consumedCount) != int64(totalFiles) {
		t.Errorf("Expected %d consumed files, got %d", totalFiles, atomic.LoadInt64(&consumedCount))
	}
	if drops := atomic.LoadInt64(&droppedCount); drops != 0 {
		t.Errorf("Expected 0 dropped jobs during startup scan, got %d", drops)
	}
}

// TestAdversarial_KeyNormalization_Comprehensive tests all variations of slashes, backslashes,
// Unicode, and Vietnamese book titles.
func TestAdversarial_KeyNormalization_Comprehensive(t *testing.T) {
	s := &Syncer{
		queue: make(chan Job, 50),
	}

	tests := []struct {
		name     string
		inputKey string
		expected string
	}{
		{"Single leading slash", "/vBook/backup.tar.zst", "vBook/backup.tar.zst"},
		{"Multiple leading slashes", "///books/novel.epub", "books/novel.epub"},
		{"Windows single backslash", "vBook\\backup\\file.zst", "vBook/backup/file.zst"},
		{"Windows nested backslashes", "vBook\\nested\\deep\\file.zst", "vBook/nested/deep/file.zst"},
		{"Mixed slashes and backslashes", "/vBook\\data//item.zst", "vBook/data//item.zst"},
		{"Vietnamese diacritics", "vbook_backup/Tru Tiên - Quyển 1.zip", "vbook_backup/Tru Tiên - Quyển 1.zip"},
		{"Vietnamese Unicode", "sách/Đấu_Phá_Khung_Thương/tập_1.epub", "sách/Đấu_Phá_Khung_Thương/tập_1.epub"},
		{"Spaces in key", "vbook/my backup file (v1).zst", "vbook/my backup file (v1).zst"},
		{"Clean POSIX key", "clean_file.txt", "clean_file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.Enqueue(Job{R2Key: tt.inputKey})
			job := <-s.queue
			if job.R2Key != tt.expected {
				t.Errorf("Key normalization failed for %q: expected %q, got %q", tt.inputKey, tt.expected, job.R2Key)
			}
		})
	}
}

// TestAdversarial_GracefulShutdownUnderActiveWork verifies that worker stops cleanly on context cancellation
// and does not panic or hang if enqueue occurs during shutdown.
func TestAdversarial_GracefulShutdownUnderActiveWork(t *testing.T) {
	mockTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Simulate network latency on upload
		time.Sleep(50 * time.Millisecond)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	tempDir, err := os.MkdirTemp("", "vbook_shutdown_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dummyFile := filepath.Join(tempDir, "test.zst")
	_ = os.WriteFile(dummyFile, []byte("content"), 0644)

	s3Client := s3.New(s3.Options{
		BaseEndpoint: aws.String("https://dummy.r2.cloudflarestorage.com"),
		Region:       "auto",
		Credentials:  credentials.NewStaticCredentialsProvider("key", "secret", ""),
		HTTPClient:   &http.Client{Transport: mockTransport},
		UsePathStyle: true,
	})

	syncer := &Syncer{
		client:  s3Client,
		bucket:  "bucket",
		baseDir: tempDir,
		queue:   make(chan Job, 64),
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start worker in background
	workerStopped := make(chan struct{})
	go func() {
		syncer.worker(ctx)
		close(workerStopped)
	}()

	// Enqueue a few jobs
	syncer.Enqueue(Job{Action: ActionUpload, LocalPath: dummyFile, R2Key: "test1.zst"})
	syncer.Enqueue(Job{Action: ActionUpload, LocalPath: dummyFile, R2Key: "test2.zst"})

	// Give worker time to start processing job 1
	time.Sleep(10 * time.Millisecond)

	// Cancel context abruptly (simulating shutdown)
	cancel()

	// Worker MUST exit promptly within 500ms
	select {
	case <-workerStopped:
		// Success: worker stopped cleanly
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Worker did not terminate within timeout after context cancellation")
	}

	// Enqueue after shutdown should not panic
	_ = syncer.Enqueue(Job{Action: ActionUpload, LocalPath: dummyFile, R2Key: "post_shutdown.zst"})
}
