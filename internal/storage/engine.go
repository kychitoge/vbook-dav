package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrNotFound         = errors.New("resource not found")
	ErrInvalidPath      = errors.New("path traversal outside storage boundary is not permitted")
	ErrPermissionDenied = errors.New("permission denied")
)

// FileItem contains metadata for a WebDAV file or directory.
type FileItem struct {
	Path        string    // Normalized virtual path starting with '/'
	Name        string    // File or directory name
	Size        int64     // File size in bytes
	IsDir       bool      // true if resource is a directory (collection)
	ModTime     time.Time // Last modification timestamp
	ContentType string    // MIME type (for example, application/octet-stream, application/epub+zip)
	ETag        string    // Unique ETag identifier
}

// StorageEngine defines the storage abstraction layer (Local Disk or Cloudflare R2 S3).
type StorageEngine interface {
	// Stat returns metadata for a file or directory.
	Stat(ctx context.Context, relPath string) (*FileItem, error)

	// List enumerates direct children of the directory at relPath.
	List(ctx context.Context, relPath string) ([]FileItem, error)

	// GetStream opens a binary read stream for the file.
	GetStream(ctx context.Context, relPath string) (io.ReadCloser, *FileItem, error)

	// PutStream writes a binary stream to the specified path.
	PutStream(ctx context.Context, relPath string, r io.Reader) (*FileItem, error)

	// MkdirAll creates a directory and any necessary ancestor directories.
	MkdirAll(ctx context.Context, relPath string) error

	// Delete removes a file or directory recursively.
	Delete(ctx context.Context, relPath string) error

	// Move renames or moves a resource.
	Move(ctx context.Context, srcPath, dstPath string) error
}
