package webdav

import (
	"bytes"
	"encoding/base64"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"vbook-dav/internal/storage/local"
)

func TestAdversarial_WebDAV_PathTraversalAttacks(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_handler_adv_*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Seed canary file inside storage
	canaryFile := filepath.Join(tempDir, "canary.txt")
	_ = os.WriteFile(canaryFile, []byte("VALID_DATA"), 0644)

	outsideDir, err := os.MkdirTemp("", "vbook_outside_*")
	if err != nil {
		t.Fatalf("MkdirTemp outside failed: %v", err)
	}
	defer os.RemoveAll(outsideDir)

	secretOutside := filepath.Join(outsideDir, "webdav_outside_secret.txt")
	_ = os.WriteFile(secretOutside, []byte("HOST_SECRET_OUTSIDE_BASEDIR"), 0644)

	st, err := local.New(tempDir)
	if err != nil {
		t.Fatalf("local.New failed: %v", err)
	}

	handler := NewHandler(st)
	authMiddleware := NewAuthMiddleware("testuser", "testpass", false)
	app := authMiddleware.Wrap(handler)

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass"))

	attackCases := []struct {
		method string
		target string
		body   string
	}{
		{"GET", "/../../webdav_outside_secret.txt", ""},
		{"PUT", "/../../webdav_outside_secret.txt", "MALICIOUS_OVERWRITE"},
		{"DELETE", "/../../webdav_outside_secret.txt", ""},
		{"PROPFIND", "/../../", ""},
		{"MKCOL", "/../../malicious_dir", ""},
		{"GET", "/..\\..\\webdav_outside_secret.txt", ""},
		{"PUT", "/..\\..\\webdav_outside_secret.txt", "MALICIOUS_OVERWRITE"},
		{"GET", "/%2e%2e/%2e%2e/webdav_outside_secret.txt", ""},
		{"PUT", "/canary.txt::$DATA", "ATTACK_STREAM"},
		{"GET", "/NUL", ""},
		{"PUT", "/NUL", "NULL_DATA"},
		{"GET", "/CON", ""},
	}

	for _, ac := range attackCases {
		t.Run(ac.method+"_"+ac.target, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if ac.body != "" {
				bodyReader = bytes.NewReader([]byte(ac.body))
			} else {
				bodyReader = bytes.NewReader(nil)
			}

			req := httptest.NewRequest(ac.method, ac.target, bodyReader)
			req.Header.Set("Authorization", authHeader)
			rec := httptest.NewRecorder()

			app.ServeHTTP(rec, req)

			// 1. Secret outside MUST NOT be leaked in response body
			if bytes.Contains(rec.Body.Bytes(), []byte("HOST_SECRET_OUTSIDE_BASEDIR")) {
				t.Fatalf("SECURITY BREACH: Response body leaked secret outside baseDir for %s %s", ac.method, ac.target)
			}

			// 2. Secret outside MUST NOT be overwritten or deleted
			data, err := os.ReadFile(secretOutside)
			if err != nil {
				t.Fatalf("SECURITY BREACH: Outside secret read error: %v by %s %s", err, ac.method, ac.target)
			}
			if !bytes.Equal(data, []byte("HOST_SECRET_OUTSIDE_BASEDIR")) {
				t.Fatalf("SECURITY BREACH: Outside secret was overwritten by %s %s", ac.method, ac.target)
			}
		})
	}
}
