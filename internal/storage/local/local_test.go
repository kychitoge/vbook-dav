package local

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"vbook-dav/internal/storage"
)

func TestLocalStorage_CRUD(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_dav_test_*")
	if err != nil {
		t.Fatalf("Không thể tạo tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	engine, err := New(tempDir)
	if err != nil {
		t.Fatalf("Lỗi khởi tạo LocalStorageEngine: %v", err)
	}

	ctx := context.Background()

	// 1. Test PutStream với Auto-MkdirAll trên đường dẫn sâu
	testContent := []byte("vBook test backup payload with 8192 bytes buffer streaming")
	item, err := engine.PutStream(ctx, "/vBook/deep/backup.tar.zst", bytes.NewReader(testContent))
	if err != nil {
		t.Fatalf("PutStream thất bại: %v", err)
	}
	if item.Size != int64(len(testContent)) {
		t.Errorf("Kỳ vọng size %d, nhận được %d", len(testContent), item.Size)
	}
	if item.ContentType != "application/zstd" {
		t.Errorf("Kỳ vọng ContentType application/zstd, nhận được %s", item.ContentType)
	}

	// 2. Test Stat
	statItem, err := engine.Stat(ctx, "/vBook/deep/backup.tar.zst")
	if err != nil {
		t.Fatalf("Stat thất bại: %v", err)
	}
	if statItem.Size != int64(len(testContent)) {
		t.Errorf("Stat size không khớp: %d vs %d", statItem.Size, len(testContent))
	}

	// 3. Test GetStream
	rc, getItem, err := engine.GetStream(ctx, "/vBook/deep/backup.tar.zst")
	if err != nil {
		t.Fatalf("GetStream thất bại: %v", err)
	}

	readData, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("Đọc stream thất bại: %v", err)
	}
	if !bytes.Equal(readData, testContent) {
		t.Errorf("Dữ liệu đọc lại không khớp với dữ liệu đã ghi")
	}
	if getItem.Name != "backup.tar.zst" {
		t.Errorf("Kỳ vọng Name backup.tar.zst, nhận được %s", getItem.Name)
	}

	// 4. Test List
	items, err := engine.List(ctx, "/vBook/deep")
	if err != nil {
		t.Fatalf("List thất bại: %v", err)
	}
	if len(items) != 1 || items[0].Name != "backup.tar.zst" {
		t.Errorf("Kết quả List không đúng: %+v", items)
	}

	// 5. Test Move
	err = engine.Move(ctx, "/vBook/deep/backup.tar.zst", "/vBook/backup_moved.tar.zst")
	if err != nil {
		t.Fatalf("Move thất bại: %v", err)
	}

	_, err = engine.Stat(ctx, "/vBook/deep/backup.tar.zst")
	if err != storage.ErrNotFound {
		t.Errorf("File cũ sau khi move phải trả về ErrNotFound, nhận được: %v", err)
	}

	movedItem, err := engine.Stat(ctx, "/vBook/backup_moved.tar.zst")
	if err != nil || movedItem.Size != int64(len(testContent)) {
		t.Errorf("File mới sau khi move không tồn tại hoặc sai dung lượng: %v", err)
	}

	// 6. Test Delete
	err = engine.Delete(ctx, "/vBook")
	if err != nil {
		t.Fatalf("Delete thất bại: %v", err)
	}
	_, err = engine.Stat(ctx, "/vBook/backup_moved.tar.zst")
	if err != storage.ErrNotFound {
		t.Errorf("Kỳ vọng ErrNotFound sau khi xóa thư mục gốc /vBook, nhận được: %v", err)
	}
}

func TestLocalStorage_PathTraversalDefense(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_dav_traversal_*")
	if err != nil {
		t.Fatalf("Không thể tạo tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	engine, err := New(tempDir)
	if err != nil {
		t.Fatalf("Lỗi khởi tạo: %v", err)
	}

	ctx := context.Background()

	// Cố ý thoát khỏi baseDir
	maliciousPaths := []string{
		"../../secret.txt",
		"/../outside",
		"..\\..\\system32",
	}

	for _, p := range maliciousPaths {
		_, err := engine.Stat(ctx, p)
		// Must return ErrInvalidPath or ErrNotFound without accessing files outside baseDir
		if err == nil {
			t.Errorf("Path traversal attempt %s should have been blocked, but succeeded", p)
		}
	}
}

type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated network connection drop")
}

func TestLocalStorage_PutStream_Interrupted(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_interrupted_*")
	if err != nil {
		t.Fatalf("failed to create tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	engine, err := New(tempDir)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx := context.Background()
	targetPath := "/vBook/interrupted.tar.zst"
	_, err = engine.PutStream(ctx, targetPath, &errorReader{})
	if err == nil {
		t.Fatalf("expected error from interrupted stream, got nil")
	}

	// Verify target file does not exist
	_, err = engine.Stat(ctx, targetPath)
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound for target file, got %v", err)
	}

	// Verify temporary file was cleaned up
	tmpDiskPath := filepath.Join(tempDir, "vBook", "interrupted.tar.zst.tmp")
	if _, err := os.Stat(tmpDiskPath); !os.IsNotExist(err) {
		t.Errorf("expected temporary file %s to be removed, but it exists", tmpDiskPath)
	}
}

func TestLocalStorage_CleanupTempFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_cleanup_*")
	if err != nil {
		t.Fatalf("failed to create tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	engine, err := New(tempDir)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Create valid files and orphan .tmp files
	subDir := filepath.Join(tempDir, "vBook")
	_ = os.MkdirAll(subDir, 0755)

	_ = os.WriteFile(filepath.Join(tempDir, "valid.txt"), []byte("data"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "stale1.tmp"), []byte("stale"), 0644)
	_ = os.WriteFile(filepath.Join(subDir, "valid2.zst"), []byte("data2"), 0644)
	_ = os.WriteFile(filepath.Join(subDir, "stale2.tmp"), []byte("stale2"), 0644)

	cleaned := engine.CleanupTempFiles()
	if cleaned != 2 {
		t.Errorf("expected 2 cleaned files, got %d", cleaned)
	}

	// Verify .tmp files were removed
	if _, err := os.Stat(filepath.Join(tempDir, "stale1.tmp")); !os.IsNotExist(err) {
		t.Errorf("expected stale1.tmp to be deleted")
	}
	if _, err := os.Stat(filepath.Join(subDir, "stale2.tmp")); !os.IsNotExist(err) {
		t.Errorf("expected stale2.tmp to be deleted")
	}

	// Verify valid files are untouched
	if _, err := os.Stat(filepath.Join(tempDir, "valid.txt")); err != nil {
		t.Errorf("expected valid.txt to remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(subDir, "valid2.zst")); err != nil {
		t.Errorf("expected valid2.zst to remain: %v", err)
	}
}

func TestLocalStorage_PutStream_AtomicOverwrite(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_overwrite_*")
	if err != nil {
		t.Fatalf("failed to create tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	engine, err := New(tempDir)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx := context.Background()
	relPath := "/vBook/backup.tar.zst"

	// Initial upload
	data1 := []byte("first upload payload")
	_, err = engine.PutStream(ctx, relPath, bytes.NewReader(data1))
	if err != nil {
		t.Fatalf("first PutStream failed: %v", err)
	}

	// Overwrite upload (verifies Windows file closure before rename)
	data2 := []byte("second upload updated payload")
	item2, err := engine.PutStream(ctx, relPath, bytes.NewReader(data2))
	if err != nil {
		t.Fatalf("second PutStream (overwrite) failed: %v", err)
	}

	if item2.Size != int64(len(data2)) {
		t.Errorf("expected size %d, got %d", len(data2), item2.Size)
	}

	// Read and verify content
	rc, _, err := engine.GetStream(ctx, relPath)
	if err != nil {
		t.Fatalf("GetStream failed: %v", err)
	}
	readData, _ := io.ReadAll(rc)
	rc.Close()

	if !bytes.Equal(readData, data2) {
		t.Errorf("content mismatch after overwrite")
	}
}
