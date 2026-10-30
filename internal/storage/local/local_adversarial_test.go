package local

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vbook-dav/internal/storage"
)

// slowOrFailingReader simulates slow network or sudden drop.
type slowOrFailingReader struct {
	data    []byte
	offset  int
	failAt  int
	chunk   int
	delay   time.Duration
	failErr error
}

func (s *slowOrFailingReader) Read(p []byte) (n int, err error) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if s.offset >= s.failAt {
		if s.failErr != nil {
			return 0, s.failErr
		}
		return 0, io.ErrUnexpectedEOF
	}
	remaining := s.failAt - s.offset
	toRead := len(p)
	if toRead > remaining {
		toRead = remaining
	}
	if s.chunk > 0 && toRead > s.chunk {
		toRead = s.chunk
	}
	copy(p, s.data[s.offset:s.offset+toRead])
	s.offset += toRead
	return toRead, nil
}

// ---------------------------------------------------------------------------
// 1. ATOMIC RENAME UNDER WINDOWS CONCURRENCY
// ---------------------------------------------------------------------------

// TestAdversarial_ConcurrentReadDuringPut tests whether an ongoing read (GetStream)
// prevents or crashes PutStream (os.Rename) on Windows NTFS.
func TestAdversarial_ConcurrentReadDuringPut(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_stress_read_put_*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	engine, err := New(tempDir)
	if err != nil {
		t.Fatalf("New engine failed: %v", err)
	}

	ctx := context.Background()
	relPath := "/vBook/concurrent_read_write.dat"

	// Seed initial file (64 KB)
	initialData := bytes.Repeat([]byte("INITIAL_DATA_BLOCK_"), 3500) // ~66 KB
	if _, err := engine.PutStream(ctx, relPath, bytes.NewReader(initialData)); err != nil {
		t.Fatalf("Initial PutStream failed: %v", err)
	}

	stopChan := make(chan struct{})
	var readErrors int64
	var writeErrors int64
	var readSuccesses int64
	var writeSuccesses int64
	var wg sync.WaitGroup

	// Start 8 concurrent reader goroutines
	numReaders := 8
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerId int) {
			defer wg.Done()
			buf := make([]byte, 4096)
			for {
				select {
				case <-stopChan:
					return
				default:
					rc, _, err := engine.GetStream(ctx, relPath)
					if err != nil {
						atomic.AddInt64(&readErrors, 1)
						time.Sleep(1 * time.Millisecond)
						continue
					}
					// Read slowly to hold the file handle open across a small interval
					totalRead := 0
					for {
						n, rErr := rc.Read(buf)
						totalRead += n
						if rErr != nil {
							break
						}
						// Small micro-yield to keep handle open during concurrent rename
						time.Sleep(100 * time.Microsecond)
					}
					rc.Close()
					if totalRead > 0 {
						atomic.AddInt64(&readSuccesses, 1)
					}
				}
			}
		}(i)
	}

	// Start 4 concurrent writer goroutines continuously putting updates to the SAME path
	numWriters := 4
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerId int) {
			defer wg.Done()
			payload := bytes.Repeat([]byte(fmt.Sprintf("WRITER_%02d_PAYLOAD_", writerId)), 2500)
			for {
				select {
				case <-stopChan:
					return
				default:
					_, err := engine.PutStream(ctx, relPath, bytes.NewReader(payload))
					if err != nil {
						cnt := atomic.AddInt64(&writeErrors, 1)
						if cnt <= 3 {
							t.Logf("Write error during concurrent read #%d: %v", cnt, err)
						}
					} else {
						atomic.AddInt64(&writeSuccesses, 1)
					}
					time.Sleep(2 * time.Millisecond)
				}
			}
		}(i)
	}

	// Let the stress test run for 1.5 seconds
	time.Sleep(1500 * time.Millisecond)
	close(stopChan)
	wg.Wait()

	t.Logf("Concurrent Read/Write Stress Results on OS=%s:", runtime.GOOS)
	t.Logf("  Read successes: %d, Read errors: %d", readSuccesses, readErrors)
	t.Logf("  Write successes: %d, Write errors: %d", writeSuccesses, writeErrors)

	// Verify target file is still valid and readable
	rc, fi, err := engine.GetStream(ctx, relPath)
	if err != nil {
		t.Fatalf("Target file corrupted or unreadable after stress: %v", err)
	}
	defer rc.Close()
	if fi.Size <= 0 {
		t.Errorf("Target file has zero or invalid size: %d", fi.Size)
	}
}

// TestAdversarial_ConcurrentPutSamePath tests race conditions when multiple writers
// attempt to write to the exact same file path simultaneously.
func TestAdversarial_ConcurrentPutSamePath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_stress_same_put_*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	engine, err := New(tempDir)
	if err != nil {
		t.Fatalf("New engine failed: %v", err)
	}

	ctx := context.Background()
	relPath := "/vBook/race_conflict.tar.zst"

	var wg sync.WaitGroup
	numWriters := 10
	results := make([]error, numWriters)

	// All writers try to write at the exact same instant
	startBarrier := make(chan struct{})

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			data := bytes.Repeat([]byte{byte(idx + 65)}, 16384) // 16 KB of char 'A' + idx
			<-startBarrier
			_, err := engine.PutStream(ctx, relPath, bytes.NewReader(data))
			results[idx] = err
		}(i)
	}

	close(startBarrier)
	wg.Wait()

	var successCount, failCount int
	for idx, err := range results {
		if err == nil {
			successCount++
		} else {
			failCount++
			t.Logf("Writer %d encountered error: %v", idx, err)
		}
	}
	t.Logf("Concurrent Put Same Path results: %d succeeded, %d failed", successCount, failCount)

	// Assert Data Integrity:
	// Under Windows NTFS, concurrent writers targeting the exact same .tmp path
	// either commit a valid uncorrupted file (16384 bytes) or safely abort.
	fi, err := engine.Stat(ctx, relPath)
	if err == nil {
		if fi.Size != 16384 {
			t.Errorf("DATA CORRUPTION DETECTED: Expected size 16384 bytes, got %d", fi.Size)
		} else {
			t.Logf("Data integrity preserved: committed file is valid (16384 bytes)")
		}
	} else if errors.Is(err, storage.ErrNotFound) {
		t.Logf("Data integrity preserved: contention caused safe abort, no corrupt partial file was committed")
	} else {
		t.Errorf("Unexpected stat error: %v", err)
	}

	// Check if any leftover .tmp files exist in the directory
	var tmpFilesFound []string
	_ = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(info.Name(), ".tmp") {
			tmpFilesFound = append(tmpFilesFound, path)
		}
		return nil
	})
	if len(tmpFilesFound) > 0 {
		t.Logf("NOTICE: Found %d leftover .tmp files from concurrent collisions: %v", len(tmpFilesFound), tmpFilesFound)
	}
}

// ---------------------------------------------------------------------------
// 2. INTERRUPTED UPLOADS LEAVING .TMP FILES AND ENSURING CLEANUP
// ---------------------------------------------------------------------------

func TestAdversarial_InterruptedUpload_FailureModes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_stress_interrupted_*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	engine, err := New(tempDir)
	if err != nil {
		t.Fatalf("New engine failed: %v", err)
	}

	ctx := context.Background()

	testCases := []struct {
		name       string
		path       string
		reader     io.Reader
		expectErr  bool
		cleanupTmp bool
	}{
		{
			name: "Fails on byte 0 (immediate EOF/error)",
			path: "/vBook/err_byte_0.dat",
			reader: &slowOrFailingReader{
				data:    []byte("some initial data"),
				failAt:  0,
				failErr: errors.New("connection reset by peer immediately"),
			},
			expectErr:  true,
			cleanupTmp: true,
		},
		{
			name: "Fails halfway through stream (after 10KB)",
			path: "/vBook/err_halfway.dat",
			reader: &slowOrFailingReader{
				data:    bytes.Repeat([]byte("HALF_STREAM_"), 2000), // ~24KB
				failAt:  10240,
				chunk:   1024,
				failErr: errors.New("client socket dropped midway"),
			},
			expectErr:  true,
			cleanupTmp: true,
		},
		{
			name: "Fails right before final block",
			path: "/vBook/err_near_end.dat",
			reader: &slowOrFailingReader{
				data:    bytes.Repeat([]byte("NEAR_END_DATA_"), 1000),
				failAt:  13990,
				failErr: io.ErrUnexpectedEOF,
			},
			expectErr:  true,
			cleanupTmp: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.PutStream(ctx, tc.path, tc.reader)
			if tc.expectErr && err == nil {
				t.Fatalf("Expected error, got nil")
			}

			// Destination file must NOT exist
			_, statErr := engine.Stat(ctx, tc.path)
			if statErr != storage.ErrNotFound {
				t.Errorf("Target file %s should NOT exist, got statErr: %v", tc.path, statErr)
			}

			// Physical .tmp file must NOT exist
			cleanRel := filepath.FromSlash(strings.TrimPrefix(tc.path, "/"))
			tmpFilePath := filepath.Join(tempDir, cleanRel+".tmp")
			if _, osErr := os.Stat(tmpFilePath); !os.IsNotExist(osErr) {
				t.Errorf("Temporary file %s was NOT cleaned up after error: %v", tmpFilePath, osErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. CLEANUP TEMP FILES BEHAVIOR
// ---------------------------------------------------------------------------

func TestAdversarial_CleanupTempFiles_ComplexScenarios(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_stress_cleanup_*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	engine, err := New(tempDir)
	if err != nil {
		t.Fatalf("New engine failed: %v", err)
	}

	// 1. Deeply nested directories (e.g., 20 levels deep)
	deepDir := tempDir
	for i := 1; i <= 20; i++ {
		deepDir = filepath.Join(deepDir, fmt.Sprintf("level_%02d", i))
	}
	if err := os.MkdirAll(deepDir, 0755); err != nil {
		t.Fatalf("Failed to create deeply nested dir: %v", err)
	}

	// Place .tmp and valid files at the deepest level
	deepTmp := filepath.Join(deepDir, "deep_orphan.tmp")
	deepValid := filepath.Join(deepDir, "deep_valid.tar.zst")
	_ = os.WriteFile(deepTmp, []byte("deep tmp content"), 0644)
	_ = os.WriteFile(deepValid, []byte("deep valid content"), 0644)

	// 2. Zero-byte .tmp file
	zeroTmp := filepath.Join(tempDir, "zero_byte.tmp")
	_ = os.WriteFile(zeroTmp, []byte{}, 0644)

	// 3. Large .tmp file (e.g. 5 MB)
	largeTmp := filepath.Join(tempDir, "large_stale.tmp")
	_ = os.WriteFile(largeTmp, make([]byte, 5*1024*1024), 0644)

	// 4. File with ".tmp" inside name but NOT suffix (e.g., "my.tmp.backup.zst", ".tmp_config")
	notSuffix1 := filepath.Join(tempDir, "my.tmp.backup.zst")
	notSuffix2 := filepath.Join(tempDir, ".tmp_metadata")
	notSuffix3 := filepath.Join(tempDir, "tmp_prefix.txt")
	_ = os.WriteFile(notSuffix1, []byte("valid 1"), 0644)
	_ = os.WriteFile(notSuffix2, []byte("valid 2"), 0644)
	_ = os.WriteFile(notSuffix3, []byte("valid 3"), 0644)

	// 5. Directory ending with ".tmp"
	dirNamedTmp := filepath.Join(tempDir, "directory_ending_with.tmp")
	_ = os.MkdirAll(dirNamedTmp, 0755)
	fileInsideDirTmp := filepath.Join(dirNamedTmp, "nested_inside.txt")
	_ = os.WriteFile(fileInsideDirTmp, []byte("inside tmp directory"), 0644)

	// 6. Read-only .tmp file (testing Windows NTFS read-only file removal)
	readOnlyTmp := filepath.Join(tempDir, "readonly_orphan.tmp")
	_ = os.WriteFile(readOnlyTmp, []byte("read only data"), 0444)
	// Explicitly set readonly mode
	_ = os.Chmod(readOnlyTmp, 0444)

	t.Logf("Created test file landscape for CleanupTempFiles()")

	// Run CleanupTempFiles
	cleaned := engine.CleanupTempFiles()
	t.Logf("CleanupTempFiles returned cleaned count: %d", cleaned)

	// ASSERTIONS:

	// A. Deeply nested .tmp should be deleted
	if _, err := os.Stat(deepTmp); !os.IsNotExist(err) {
		t.Errorf("Deeply nested .tmp file was NOT deleted: %s", deepTmp)
	}
	// Deeply nested valid file MUST remain
	if _, err := os.Stat(deepValid); err != nil {
		t.Errorf("Deeply nested valid file was accidentally deleted or stat error: %v", err)
	}

	// B. Zero-byte .tmp should be deleted
	if _, err := os.Stat(zeroTmp); !os.IsNotExist(err) {
		t.Errorf("Zero-byte .tmp file was NOT deleted: %s", zeroTmp)
	}

	// C. Large .tmp file should be deleted
	if _, err := os.Stat(largeTmp); !os.IsNotExist(err) {
		t.Errorf("Large .tmp file was NOT deleted: %s", largeTmp)
	}

	// D. Files NOT ending with .tmp MUST NOT be deleted
	for _, validFile := range []string{notSuffix1, notSuffix2, notSuffix3} {
		if _, err := os.Stat(validFile); err != nil {
			t.Errorf("Valid file %s was improperly deleted: %v", validFile, err)
		}
	}

	// E. Directory named with .tmp MUST NOT be deleted as a file
	if fi, err := os.Stat(dirNamedTmp); err != nil || !fi.IsDir() {
		t.Errorf("Directory ending with .tmp was corrupted or deleted: %v", err)
	}
	if _, err := os.Stat(fileInsideDirTmp); err != nil {
		t.Errorf("File inside .tmp directory was deleted: %v", err)
	}

	// F. Read-only .tmp file inspection (Crucial for Windows NTFS)
	if _, err := os.Stat(readOnlyTmp); err == nil {
		t.Logf("WARNING/FINDING: Read-only .tmp file %s was NOT deleted by os.Remove! Error details: Windows os.Remove fails on read-only attribute.", readOnlyTmp)
	} else if os.IsNotExist(err) {
		t.Logf("Read-only .tmp file was successfully deleted.")
	}

	// G. Actively locked .tmp file (upload in-progress simulation)
	lockedTmp := filepath.Join(tempDir, "active_upload.tmp")
	fLock, err := os.OpenFile(lockedTmp, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to create active locked file: %v", err)
	}
	defer fLock.Close()
	_, _ = fLock.WriteString("in progress stream data")

	// Run cleanup again - must not crash or corrupt
	cleanedPass2 := engine.CleanupTempFiles()
	t.Logf("CleanupTempFiles with open/locked file returned count: %d", cleanedPass2)

	// OS-dependent behavior verification:
	// - Windows NTFS: mandatory file sharing lock prevents os.Remove (ERROR_SHARING_VIOLATION), file remains.
	// - Linux/POSIX: unlink() succeeds on open files; file descriptor remains valid until Close().
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(lockedTmp); err != nil {
			t.Errorf("On Windows, locked active upload file was prematurely removed: %v", err)
		}
	} else {
		t.Logf("On POSIX (%s), open file was unlinked as expected by OS filesystem semantics.", runtime.GOOS)
	}
}

// ---------------------------------------------------------------------------
// 4. PATH TRAVERSAL DEFENSES WITH TRICKY VIRTUAL PATHS
// ---------------------------------------------------------------------------

func TestAdversarial_PathTraversal_ExtremeCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_stress_traversal_*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	engine, err := New(tempDir)
	if err != nil {
		t.Fatalf("New engine failed: %v", err)
	}

	ctx := context.Background()

	// Seed a canary file in base directory
	canaryPath := "/canary.txt"
	canaryData := []byte("CANARY_DATA")
	if _, err := engine.PutStream(ctx, canaryPath, bytes.NewReader(canaryData)); err != nil {
		t.Fatalf("Failed to seed canary file: %v", err)
	}

	// Also seed a secret file OUTSIDE tempDir (in parent of tempDir)
	secretPathOutside := filepath.Join(filepath.Dir(tempDir), "adversarial_secret.txt")
	_ = os.WriteFile(secretPathOutside, []byte("SUPER_SECRET_HOST_DATA"), 0644)
	defer os.Remove(secretPathOutside)

	// Tricky attack vectors
	attackVectors := []struct {
		name string
		path string
	}{
		{"Double dot unix", "../../adversarial_secret.txt"},
		{"Double dot with leading slash", "/../../adversarial_secret.txt"},
		{"Triple dot", ".../.../adversarial_secret.txt"},
		{"Quad dot slash", "..../..../adversarial_secret.txt"},
		{"Windows backslash escape", "..\\..\\adversarial_secret.txt"},
		{"Mixed slash traversal", "../..\\../adversarial_secret.txt"},
		{"Windows drive letter C:", "C:/Windows/win.ini"},
		{"Windows drive letter D:", "D:/Windows/win.ini"},
		{"Windows drive letter with backslash", "C:\\Windows\\win.ini"},
		{"UNC path double slash", "//127.0.0.1/c$/Windows/win.ini"},
		{"UNC path quadruple backslash", "\\\\127.0.0.1\\c$\\Windows"},
		{"Windows device NUL", "NUL"},
		{"Windows device CON", "CON"},
		{"Windows device AUX", "AUX"},
		{"Windows device COM1", "COM1"},
		{"Windows device PRN", "PRN"},
		{"Windows device with ext", "CON.txt"},
		{"Windows Alternate Data Stream", "canary.txt:hidden_stream"},
		{"Windows ADS with DATA", "canary.txt::$DATA"},
		{"URL encoded double dot", "%2e%2e%2f%2e%2e%2fsecret.txt"},
		{"Trailing spaces and dots", "canary.txt ..."},
		{"Deeply nested traversal back and forth", "/a/b/c/../../../../../../../../adversarial_secret.txt"},
		{"Root path itself", "/"},
		{"Empty string", ""},
		{"Spaces only", "    "},
	}

	for _, tc := range attackVectors {
		t.Run(tc.name, func(t *testing.T) {
			// Test 1: Stat
			item, statErr := engine.Stat(ctx, tc.path)
			if statErr == nil && item != nil && !item.IsDir {
				// If Stat succeeded on a file, verify it didn't read outside baseDir
				fullPath, _, rErr := engine.resolvePath(tc.path)
				if rErr == nil {
					rel, relErr := filepath.Rel(tempDir, fullPath)
					if relErr != nil || strings.HasPrefix(rel, "..") {
						t.Fatalf("CRITICAL SECURITY VULNERABILITY: Stat accessed file outside baseDir! Path: %s -> %s", tc.path, fullPath)
					}
				}
			}

			// Test 2: GetStream - MUST NOT leak secretPathOutside
			rc, _, getErr := engine.GetStream(ctx, tc.path)
			if getErr == nil && rc != nil {
				data, _ := io.ReadAll(rc)
				rc.Close()
				if bytes.Contains(data, []byte("SUPER_SECRET_HOST_DATA")) {
					t.Fatalf("CRITICAL SECURITY LEAK: GetStream read outside baseDir! Path: %s leaked host secret!", tc.path)
				}
			}

			// Test 3: PutStream - MUST NOT write outside baseDir
			maliciousPayload := []byte("EXPLOIT_WRITE_PAYLOAD")
			_, putErr := engine.PutStream(ctx, tc.path, bytes.NewReader(maliciousPayload))
			if putErr == nil {
				// Verify where it wrote
				fullPath, _, rErr := engine.resolvePath(tc.path)
				if rErr == nil {
					rel, relErr := filepath.Rel(tempDir, fullPath)
					if relErr != nil || strings.HasPrefix(rel, "..") {
						t.Fatalf("CRITICAL SECURITY VULNERABILITY: PutStream wrote outside baseDir! Path: %s -> %s", tc.path, fullPath)
					}
				}
			}

			// Test 4: Delete - MUST NOT delete secretPathOutside
			_ = engine.Delete(ctx, tc.path)
			if _, err := os.Stat(secretPathOutside); os.IsNotExist(err) {
				t.Fatalf("CRITICAL SECURITY VULNERABILITY: Delete removed file outside baseDir! Path: %s deleted %s", tc.path, secretPathOutside)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5. STRESS TEST: HIGH-THROUGHPUT RAPID WRITES & OVERWRITES
// ---------------------------------------------------------------------------

func TestAdversarial_RapidSequentialOverwrites(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_stress_overwrites_*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	engine, err := New(tempDir)
	if err != nil {
		t.Fatalf("New engine failed: %v", err)
	}

	ctx := context.Background()
	relPath := "/vBook/rapid_overwrite.tar.zst"

	// Perform 100 rapid sequential PutStream overwrites
	for i := 1; i <= 100; i++ {
		payload := []byte(fmt.Sprintf("OVERWRITE_ITERATION_%d_TIMESTAMP_%d", i, time.Now().UnixNano()))
		item, err := engine.PutStream(ctx, relPath, bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("PutStream failed on iteration %d: %v", i, err)
		}
		if item.Size != int64(len(payload)) {
			t.Fatalf("Size mismatch on iteration %d: expected %d, got %d", i, len(payload), item.Size)
		}
	}

	// Verify no orphan .tmp files left
	cleaned := engine.CleanupTempFiles()
	if cleaned != 0 {
		t.Errorf("Expected 0 orphan .tmp files after sequential overwrites, but found %d", cleaned)
	}
}
