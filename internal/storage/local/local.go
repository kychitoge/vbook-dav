package local

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"

	"vbook-dav/internal/storage"
)

const (
	streamBufferSize = 8192 // Standard 8 KB chunk size per vBook WebDAV contract
	dirPermissions   = 0755
	filePermissions  = 0644
)

// LocalStorageEngine implements StorageEngine directly on the host filesystem.
type LocalStorageEngine struct {
	baseDir string
}

// New creates a LocalStorageEngine instance with the specified base directory.
func New(baseDir string) (*LocalStorageEngine, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to determine absolute path for base directory: %w", err)
	}

	if err := os.MkdirAll(absBase, dirPermissions); err != nil {
		return nil, fmt.Errorf("failed to create storage directory %s: %w", absBase, err)
	}

	return &LocalStorageEngine{
		baseDir: absBase,
	}, nil
}

// isWindowsReservedName checks if a filename segment matches a Windows reserved device name.
func isWindowsReservedName(name string) bool {
	upper := strings.ToUpper(strings.TrimSpace(name))
	switch upper {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}

// resolvePath normalizes a virtual path to a physical filesystem path and defends against directory traversal.
func (l *LocalStorageEngine) resolvePath(relPath string) (string, string, error) {
	cleanRel := path.Clean("/" + strings.TrimSpace(relPath))

	// Guard against NTFS Alternate Data Streams (colon separator)
	if strings.Contains(cleanRel, ":") {
		return "", "", storage.ErrInvalidPath
	}

	// Guard against Windows DOS reserved device names (CON, PRN, AUX, NUL, COM1-9, LPT1-9)
	for _, segment := range strings.Split(cleanRel, "/") {
		if segment == "" {
			continue
		}
		base := segment
		if idx := strings.IndexByte(segment, '.'); idx != -1 {
			base = segment[:idx]
		}
		if isWindowsReservedName(base) {
			return "", "", storage.ErrInvalidPath
		}
	}

	// Convert virtual forward slashes to OS-specific path separators
	osRel := filepath.FromSlash(strings.TrimPrefix(cleanRel, "/"))
	fullPath := filepath.Join(l.baseDir, osRel)

	// Guard against path traversal escaping baseDir
	relToRoot, err := filepath.Rel(l.baseDir, fullPath)
	if err != nil || strings.HasPrefix(relToRoot, "..") {
		return "", "", storage.ErrInvalidPath
	}

	return fullPath, cleanRel, nil
}

// Stat checks existence and returns metadata for a resource.
func (l *LocalStorageEngine) Stat(ctx context.Context, relPath string) (*storage.FileItem, error) {
	fullPath, cleanRel, err := l.resolvePath(relPath)
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	return l.buildFileItem(cleanRel, fi), nil
}

// List returns direct child files and directories within relPath.
func (l *LocalStorageEngine) List(ctx context.Context, relPath string) ([]storage.FileItem, error) {
	fullPath, cleanRel, err := l.resolvePath(relPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	items := make([]storage.FileItem, 0, len(entries))
	for _, entry := range entries {
		// Ignore hidden files (.gitkeep, .DS_Store) and internal staging files (*.tmp)
		if strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}

		itemPath := path.Join(cleanRel, entry.Name())
		items = append(items, *l.buildFileItem(itemPath, info))
	}

	return items, nil
}

// GetStream opens a binary read stream for a file.
func (l *LocalStorageEngine) GetStream(ctx context.Context, relPath string) (io.ReadCloser, *storage.FileItem, error) {
	fullPath, cleanRel, err := l.resolvePath(relPath)
	if err != nil {
		return nil, nil, err
	}

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, storage.ErrNotFound
		}
		return nil, nil, err
	}

	fi, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}

	if fi.IsDir() {
		file.Close()
		return nil, nil, fmt.Errorf("cannot read stream from directory: %w", storage.ErrPermissionDenied)
	}

	return file, l.buildFileItem(cleanRel, fi), nil
}

// PutStream writes a binary stream to a temporary file, flushes to disk,
// closes the file handle, and atomically renames it to the destination.
func (l *LocalStorageEngine) PutStream(ctx context.Context, relPath string, r io.Reader) (*storage.FileItem, error) {
	fullPath, cleanRel, err := l.resolvePath(relPath)
	if err != nil {
		return nil, err
	}

	if cleanRel == "/" || cleanRel == "" {
		return nil, storage.ErrInvalidPath
	}

	// Auto-MkdirAll: ensure ancestor directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return nil, fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Atomic Rename pattern: stage upload in temporary file
	tmpPath := fullPath + ".tmp"
	dstFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, filePermissions)
	if err != nil {
		return nil, fmt.Errorf("failed to open temporary file: %w", err)
	}

	// Stream via 8192-byte buffer per vBook protocol contract
	buf := make([]byte, streamBufferSize)
	if _, err := io.CopyBuffer(dstFile, r, buf); err != nil {
		dstFile.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("stream interrupted: %w", err)
	}

	// Flush buffer to physical storage
	if err := dstFile.Sync(); err != nil {
		dstFile.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to flush data: %w", err)
	}

	// Crucial on Windows: file handle must be closed before os.Rename to avoid ERROR_SHARING_VIOLATION
	if err := dstFile.Close(); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Atomically replace target destination
	if err := os.Rename(tmpPath, fullPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to finalize file: %w", err)
	}

	fi, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect finalized file: %w", err)
	}

	return l.buildFileItem(cleanRel, fi), nil
}

// CleanupTempFiles removes leftover .tmp files from interrupted uploads.
func (l *LocalStorageEngine) CleanupTempFiles() int {
	count := 0
	filepath.WalkDir(l.baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".tmp") {
			if err := os.Remove(path); err == nil {
				count++
			}
		}
		return nil
	})
	return count
}

// MkdirAll creates a directory and any necessary parents.
func (l *LocalStorageEngine) MkdirAll(ctx context.Context, relPath string) error {
	fullPath, _, err := l.resolvePath(relPath)
	if err != nil {
		return err
	}

	return os.MkdirAll(fullPath, dirPermissions)
}

// Delete removes a file or directory recursively.
func (l *LocalStorageEngine) Delete(ctx context.Context, relPath string) error {
	fullPath, cleanRel, err := l.resolvePath(relPath)
	if err != nil {
		return err
	}

	if cleanRel == "/" || cleanRel == "" {
		return storage.ErrPermissionDenied
	}

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return storage.ErrNotFound
	}

	return os.RemoveAll(fullPath)
}

// Move renames or moves a resource.
func (l *LocalStorageEngine) Move(ctx context.Context, srcPath, dstPath string) error {
	fullSrc, cleanSrc, err := l.resolvePath(srcPath)
	if err != nil {
		return err
	}

	fullDst, cleanDst, err := l.resolvePath(dstPath)
	if err != nil {
		return err
	}

	if cleanSrc == "/" || cleanDst == "/" {
		return storage.ErrPermissionDenied
	}

	if _, err := os.Stat(fullSrc); os.IsNotExist(err) {
		return storage.ErrNotFound
	}

	// Ensure destination parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullDst), dirPermissions); err != nil {
		return err
	}

	return os.Rename(fullSrc, fullDst)
}

// buildFileItem converts os.FileInfo to storage.FileItem metadata.
func (l *LocalStorageEngine) buildFileItem(webPath string, fi os.FileInfo) *storage.FileItem {
	isDir := fi.IsDir()
	size := fi.Size()
	name := fi.Name()
	modTime := fi.ModTime()

	etag := fmt.Sprintf(`"%x-%x"`, modTime.UnixNano(), size)
	contentType := "application/octet-stream"

	if isDir {
		contentType = "httpd/unix-directory"
	} else {
		ext := strings.ToLower(path.Ext(name))
		switch ext {
		case ".zst":
			contentType = "application/zstd"
		case ".tar":
			contentType = "application/x-tar"
		case ".zip":
			contentType = "application/zip"
		case ".epub":
			contentType = "application/epub+zip"
		case ".txt":
			contentType = "text/plain; charset=utf-8"
		case ".pdf":
			contentType = "application/pdf"
		case ".json":
			contentType = "application/json"
		default:
			mimeType := mime.TypeByExtension(ext)
			if mimeType != "" {
				contentType = mimeType
			}
		}
	}

	return &storage.FileItem{
		Path:        webPath,
		Name:        name,
		Size:        size,
		IsDir:       isDir,
		ModTime:     modTime,
		ContentType: contentType,
		ETag:        etag,
	}
}
