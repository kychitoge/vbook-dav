package webdav

import (
	"crypto/subtle"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
	ansiRed     = "\033[31m"
	ansiGray    = "\033[90m"
)

func useColor() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return true
}

// countingReader wraps an io.ReadCloser to track incoming payload stream size.
type countingReader struct {
	io.ReadCloser
	bytesRead int64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.ReadCloser.Read(p)
	cr.bytesRead += int64(n)
	return n, err
}

// responseWriterWrapper wraps http.ResponseWriter to track response status code and written byte count.
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
	bytesCount int64
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesCount += int64(n)
	return n, err
}

// AuthMiddleware manages Basic Authentication, audit logging, and OPTIONS bypass for vBook.
type AuthMiddleware struct {
	expectedUser string
	expectedPass string
	readOnly     bool
}

// NewAuthMiddleware creates a new authentication middleware instance.
func NewAuthMiddleware(user, pass string, readOnly bool) *AuthMiddleware {
	return &AuthMiddleware{
		expectedUser: user,
		expectedPass: pass,
		readOnly:     readOnly,
	}
}

// Wrap wraps an http.Handler with Basic Authentication and Microsoft Style Guide audit logging.
func (a *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &responseWriterWrapper{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // Default if WriteHeader is not explicitly invoked
		}

		// Health check endpoint for Docker and container orchestration
		if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok\n"))
			return
		}

		// vBook Quirk 3: Bypass authentication for OPTIONS during client connectivity checks
		if r.Method == http.MethodOptions {
			next.ServeHTTP(wrapped, r)
			a.logRequest(r, wrapped.statusCode, start, wrapped.bytesCount)
			return
		}

		// Basic Authentication verification
		user, pass, ok := r.BasicAuth()
		if !ok || !a.checkCredentials(user, pass) {
			wrapped.Header().Set("WWW-Authenticate", `Basic realm="vBook WebDAV"`)
			wrapped.WriteHeader(http.StatusUnauthorized)
			wrapped.Write([]byte("Authentication required. Enter the WebDAV username and password configured for this server.\n"))

			a.logRequest(r, wrapped.statusCode, start, wrapped.bytesCount)
			return
		}

		// Read-only server defense
		if a.readOnly && isWriteMethod(r.Method) {
			wrapped.WriteHeader(http.StatusForbidden)
			wrapped.Write([]byte("Write operations are disabled. The server is operating in read-only mode.\n"))

			a.logRequest(r, wrapped.statusCode, start, wrapped.bytesCount)
			return
		}

		// Track incoming stream size and print real-time start notice for long uploads
		var cr *countingReader
		if r.Body != nil && (r.Method == http.MethodPut || r.Method == http.MethodPost) {
			cr = &countingReader{ReadCloser: r.Body}
			r.Body = cr

			clientIP := extractClientIP(r.RemoteAddr)
			sizeHint := ""
			if r.ContentLength > 0 {
				sizeHint = fmt.Sprintf(" (~%s)", formatSize(r.ContentLength))
			}
			if useColor() {
				log.Printf("%s[info ]%s Receiving %s%s from %s...",
					ansiCyan, ansiReset, r.URL.Path, sizeHint, clientIP)
			} else {
				log.Printf("[info ] Receiving %s%s from %s...",
					r.URL.Path, sizeHint, clientIP)
			}
		}

		// Forward to WebDAV Handler
		next.ServeHTTP(wrapped, r)

		bytesTransferred := wrapped.bytesCount
		if cr != nil && cr.bytesRead > 0 {
			bytesTransferred = cr.bytesRead
		} else if r.Method == http.MethodPut && r.ContentLength > 0 && bytesTransferred == 0 {
			bytesTransferred = r.ContentLength
		}

		a.logRequest(r, wrapped.statusCode, start, bytesTransferred)
	})
}

// checkCredentials performs constant-time string comparison to resist timing attacks.
func (a *AuthMiddleware) checkCredentials(user, pass string) bool {
	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(a.expectedUser)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(a.expectedPass)) == 1
	return userMatch && passMatch
}

func (a *AuthMiddleware) logRequest(r *http.Request, statusCode int, start time.Time, bytesCount int64) {
	duration := time.Since(start)
	durStr := formatDuration(duration)
	sizeStr := formatSize(bytesCount)

	speedStr := ""
	if duration >= 500*time.Millisecond && bytesCount >= 1024 {
		speed := float64(bytesCount) / duration.Seconds()
		speedStr = fmt.Sprintf(" @ %s/s", formatSize(int64(speed)))
	}

	remoteIP := extractClientIP(r.RemoteAddr)

	// Determine normalized 5-character tag
	tag := "ok   "
	if statusCode == http.StatusUnauthorized {
		tag = "auth "
	} else if statusCode == http.StatusForbidden || (statusCode >= 400 && statusCode < 500) {
		tag = "warn "
	} else if statusCode >= 500 {
		tag = "fail "
	}

	if useColor() {
		tagColor := ansiGreen
		switch tag {
		case "auth ":
			tagColor = ansiYellow
		case "warn ":
			tagColor = ansiYellow
		case "fail ":
			tagColor = ansiRed
		}

		statusColor := ansiGreen
		if statusCode >= 400 && statusCode < 500 {
			statusColor = ansiYellow
		} else if statusCode >= 500 {
			statusColor = ansiRed
		}

		methodColor := ansiCyan
		switch r.Method {
		case http.MethodPut:
			methodColor = ansiGreen
		case http.MethodGet:
			methodColor = ansiBlue
		case http.MethodDelete:
			methodColor = ansiMagenta
		case "MKCOL", "MOVE":
			methodColor = ansiYellow
		case http.MethodOptions, http.MethodHead:
			methodColor = ansiGray
		}

		log.Printf("%s[%s]%s %s%-8s%s %-32s <- %-15s %s%d%s (%s, %s%s)",
			tagColor, tag, ansiReset,
			methodColor, r.Method, ansiReset,
			r.URL.Path,
			remoteIP,
			statusColor, statusCode, ansiReset,
			sizeStr, durStr, speedStr,
		)
		return
	}

	log.Printf("[%s] %-8s %-32s <- %-15s %d (%s, %s%s)",
		tag,
		r.Method,
		r.URL.Path,
		remoteIP,
		statusCode,
		sizeStr,
		durStr,
		speedStr,
	)
}

func extractClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	return remoteAddr
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000.0)
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// formatSize formats byte counts into human-readable binary units (B, KB, MB, GB).
func formatSize(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPut, http.MethodDelete, "MKCOL", "MOVE", "COPY", "PROPPATCH":
		return true
	default:
		return false
	}
}
