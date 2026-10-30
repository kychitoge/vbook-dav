package webdav

import (
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"vbook-dav/internal/storage"
)

const (
	streamBufferSize = 8192 // Standard 8 KB chunk size matching vBook WebDAV contract
)

// Handler dispatches WebDAV requests compliant with RFC 4918 and vBook application quirks.
type Handler struct {
	storage  storage.StorageEngine
	onPut    func(relPath string)
	onDelete func(relPath string)
}

// NewHandler initializes a Handler with the given StorageEngine.
func NewHandler(st storage.StorageEngine) *Handler {
	return &Handler{
		storage: st,
	}
}

// SetSyncHooks registers optional asynchronous callbacks for write mutations.
func (h *Handler) SetSyncHooks(onPut func(relPath string), onDelete func(relPath string)) {
	h.onPut = onPut
	h.onDelete = onDelete
}

// ServeHTTP routes incoming requests based on the HTTP method.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cleanPath := path.Clean("/" + r.URL.Path)

	switch r.Method {
	case http.MethodOptions:
		h.handleOptions(w, r)
	case http.MethodHead:
		h.handleHead(w, r, cleanPath)
	case http.MethodGet:
		h.handleGet(w, r, cleanPath)
	case http.MethodPut:
		h.handlePut(w, r, cleanPath)
	case "MKCOL":
		h.handleMkcol(w, r, cleanPath)
	case "PROPFIND":
		h.handlePropfind(w, r, cleanPath)
	case http.MethodDelete:
		h.handleDelete(w, r, cleanPath)
	case "MOVE":
		h.handleMove(w, r, cleanPath)
	case "PROPPATCH":
		h.handleProppatch(w, r, cleanPath)
	default:
		w.Header().Set("Allow", "OPTIONS, GET, HEAD, PUT, DELETE, MKCOL, PROPFIND, PROPPATCH, MOVE")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// Quirk 3: OPTIONS allows vBook connection pre-flight checks without authentication.
func (h *Handler) handleOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("DAV", "1, 2")
	w.Header().Set("MS-Author-Via", "DAV")
	w.Header().Set("Allow", "OPTIONS, GET, HEAD, PUT, DELETE, MKCOL, PROPFIND, PROPPATCH, MOVE, COPY")
	w.WriteHeader(http.StatusOK)
}

// HEAD checks resource existence per Section 2.2 of the specification.
func (h *Handler) handleHead(w http.ResponseWriter, r *http.Request, reqPath string) {
	item, err := h.storage.Stat(r.Context(), reqPath)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// Contract Section 2.2: If not found, return 404 Not Found so vBook triggers MKCOL
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.setFileHeaders(w, item)
	w.WriteHeader(http.StatusOK)
}

// GET serves file downloads as binary streams or returns diagnostic page for directories.
func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request, reqPath string) {
	rc, item, err := h.storage.GetStream(r.Context(), reqPath)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, storage.ErrPermissionDenied) {
			// Directory diagnosis page for phone browser connectivity verification
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			escapedPath := html.EscapeString(reqPath)
			htmlContent := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>vbook-dav Gateway</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #09090b; color: #f4f4f5; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; padding: 24px; box-sizing: border-box; }
    .card { background: #18181b; border: 1px solid #27272a; border-radius: 6px; padding: 32px; max-width: 480px; width: 100%; }
    .status { display: inline-flex; align-items: center; gap: 8px; font-size: 13px; color: #22c55e; margin-bottom: 16px; font-weight: 500; }
    .dot { width: 8px; height: 8px; border-radius: 50%; background: #22c55e; }
    h1 { font-size: 20px; font-weight: 600; margin: 0 0 8px 0; color: #fafafa; }
    p { font-size: 14px; line-height: 1.5; color: #a1a1aa; margin: 0 0 16px 0; }
    .path-label { font-size: 12px; color: #71717a; margin-bottom: 6px; }
    .path-box { background: #27272a; border-radius: 4px; padding: 10px 14px; font-family: monospace; font-size: 13px; color: #38bdf8; word-break: break-all; }
  </style>
</head>
<body>
  <div class="card">
    <div class="status"><span class="dot"></span> Gateway operational</div>
    <h1>vbook-dav</h1>
    <p>WebDAV service is online and ready for vBook synchronization.</p>
    <div class="path-label">Requested directory</div>
    <div class="path-box">` + escapedPath + `</div>
  </div>
</body>
</html>`
			_, _ = w.Write([]byte(htmlContent))
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	h.setFileHeaders(w, item)
	w.WriteHeader(http.StatusOK)

	buf := make([]byte, streamBufferSize)
	_, _ = io.CopyBuffer(w, rc, buf)
}

// Quirk 2: PUT automatically creates ancestor directories (Auto-MkdirAll) and streams using an 8 KB buffer.
func (h *Handler) handlePut(w http.ResponseWriter, r *http.Request, reqPath string) {
	if reqPath == "/" || reqPath == "" {
		http.Error(w, "Cannot overwrite root collection with a file", http.StatusMethodNotAllowed)
		return
	}

	parentDir := path.Dir(reqPath)
	if parentDir != "/" && parentDir != "." {
		if err := h.storage.MkdirAll(r.Context(), parentDir); err != nil {
			http.Error(w, "Failed to create parent directory: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	item, err := h.storage.PutStream(r.Context(), reqPath, r.Body)
	if err != nil {
		http.Error(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if h.onPut != nil {
		go h.onPut(reqPath)
	}

	w.Header().Set("ETag", item.ETag)
	w.WriteHeader(http.StatusCreated)
}

// Quirk 1: Idempotent MKCOL returns 201 Created if the target directory already exists.
func (h *Handler) handleMkcol(w http.ResponseWriter, r *http.Request, reqPath string) {
	// Check whether the directory already exists.
	existing, err := h.storage.Stat(r.Context(), reqPath)
	if err == nil {
		if existing.IsDir {
			// Collection already exists; return 201 Created for vBook sync continuation.
			w.WriteHeader(http.StatusCreated)
			return
		}
		// Resource exists but is a regular file.
		http.Error(w, "Resource exists and is not a collection", http.StatusMethodNotAllowed)
		return
	}

	// Collection does not exist; create it including parent directories if needed.
	if err := h.storage.MkdirAll(r.Context(), reqPath); err != nil {
		http.Error(w, "Failed to create collection: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// PROPFIND lists resources and metadata (Depth: 0 or 1).
func (h *Handler) handlePropfind(w http.ResponseWriter, r *http.Request, reqPath string) {
	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "1"
	}

	statItem, err := h.storage.Stat(r.Context(), reqPath)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ms := NewMultiStatus()
	ms.AddFileItem(*statItem)

	if statItem.IsDir && depth != "0" {
		children, err := h.storage.List(r.Context(), reqPath)
		if err == nil {
			for _, child := range children {
				ms.AddFileItem(child)
			}
		}
	}

	xmlBytes, err := ms.RenderXML()
	if err != nil {
		http.Error(w, "Failed to render XML: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(207) // 207 Multi-Status
	_, _ = w.Write(xmlBytes)
}

// DELETE removes a file or directory recursively.
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request, reqPath string) {
	if reqPath == "/" || reqPath == "" {
		http.Error(w, "Cannot delete root collection", http.StatusForbidden)
		return
	}

	err := h.storage.Delete(r.Context(), reqPath)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if h.onDelete != nil {
		go h.onDelete(reqPath)
	}

	w.WriteHeader(http.StatusNoContent)
}

// MOVE renames or moves a resource.
func (h *Handler) handleMove(w http.ResponseWriter, r *http.Request, reqPath string) {
	dest := r.Header.Get("Destination")
	if dest == "" {
		http.Error(w, "Missing Destination header", http.StatusBadRequest)
		return
	}

	u, err := url.Parse(dest)
	if err != nil {
		http.Error(w, "Invalid Destination header", http.StatusBadRequest)
		return
	}

	targetPath := path.Clean("/" + u.Path)

	if reqPath == "/" || targetPath == "/" {
		http.Error(w, "Cannot move root collection", http.StatusForbidden)
		return
	}

	if err := h.storage.Move(r.Context(), reqPath, targetPath); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "Source not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// PROPPATCH returns 207 Multi-Status stub for client compatibility.
func (h *Handler) handleProppatch(w http.ResponseWriter, r *http.Request, reqPath string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(207)
	resp := `<?xml version="1.0" encoding="utf-8"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>` + reqPath + `</d:href><d:propstat><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`
	_, _ = w.Write([]byte(resp))
}

func (h *Handler) setFileHeaders(w http.ResponseWriter, item *storage.FileItem) {
	w.Header().Set("ETag", item.ETag)
	w.Header().Set("Last-Modified", item.ModTime.UTC().Format(http.TimeFormat))
	if !item.IsDir {
		w.Header().Set("Content-Type", item.ContentType)
		w.Header().Set("Content-Length", strconv.FormatInt(item.Size, 10))
	} else {
		w.Header().Set("Content-Type", "httpd/unix-directory")
	}
}
