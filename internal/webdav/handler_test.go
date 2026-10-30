package webdav

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"vbook-dav/internal/storage/local"
)

func setupTestServer(t *testing.T) (http.Handler, string) {
	tempDir, err := os.MkdirTemp("", "vbook_handler_test_*")
	if err != nil {
		t.Fatalf("Không thể tạo tempDir: %v", err)
	}

	st, err := local.New(tempDir)
	if err != nil {
		t.Fatalf("Không thể khởi tạo LocalStorage: %v", err)
	}

	handler := NewHandler(st)
	authMiddleware := NewAuthMiddleware("testuser", "testpass", false)

	return authMiddleware.Wrap(handler), tempDir
}

// 1. Kiểm tra Dị Biệt 3: OPTIONS bypass xác thực (nút "Kiểm tra kết nối" của vBook)
func TestQuirk_OptionsBypassAuth(t *testing.T) {
	app, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	// Hoàn toàn không kèm Authorization header
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("OPTIONS kỳ vọng HTTP 200 OK, nhận được: %d", rec.Code)
	}

	davHeader := rec.Header().Get("DAV")
	if !strings.Contains(davHeader, "1") {
		t.Errorf("Kỳ vọng header DAV chứa '1', nhận được: %s", davHeader)
	}

	allowHeader := rec.Header().Get("Allow")
	if !strings.Contains(allowHeader, "MKCOL") || !strings.Contains(allowHeader, "PROPFIND") {
		t.Errorf("Header Allow thiếu phương thức: %s", allowHeader)
	}
}

// 2. Kiểm tra Dị Biệt 1: Idempotent MKCOL (Tạo thư mục đã có vẫn trả 201 Created)
func TestQuirk_IdempotentMKCOL(t *testing.T) {
	app, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass"))

	// Lần 1: Tạo thư mục mới
	req1 := httptest.NewRequest("MKCOL", "/vBook", nil)
	req1.Header.Set("Authorization", authHeader)
	rec1 := httptest.NewRecorder()
	app.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusCreated {
		t.Errorf("MKCOL lần 1 kỳ vọng 201 Created, nhận được: %d", rec1.Code)
	}

	// Lần 2: vBook gọi lại MKCOL trên thư mục đã có
	req2 := httptest.NewRequest("MKCOL", "/vBook", nil)
	req2.Header.Set("Authorization", authHeader)
	rec2 := httptest.NewRecorder()
	app.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusCreated {
		t.Errorf("Dị biệt MKCOL lần 2: Kỳ vọng 201 Created (idempotent), nhận được: %d", rec2.Code)
	}
}

// 3. Kiểm tra Dị Biệt 2: Auto-MkdirAll trên PUT vào đường dẫn sâu
func TestQuirk_AutoMkdirAllOnPUT(t *testing.T) {
	app, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass"))

	// PUT vào /vBook/nested/level2/backup.tar.zst khi chưa hề tạo nested hay level2
	payload := []byte("vBook payload data zstandard compressed")
	req := httptest.NewRequest(http.MethodPut, "/vBook/nested/level2/backup.tar.zst", bytes.NewReader(payload))
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT kỳ vọng 201 Created, nhận được: %d. Body: %s", rec.Code, rec.Body.String())
	}

	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Errorf("PUT thành công phải có ETag")
	}

	// GET lại để kiểm chứng nội dung
	getReq := httptest.NewRequest(http.MethodGet, "/vBook/nested/level2/backup.tar.zst", nil)
	getReq.Header.Set("Authorization", authHeader)
	getRec := httptest.NewRecorder()

	app.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET kỳ vọng 200 OK, nhận được: %d", getRec.Code)
	}

	if !bytes.Equal(getRec.Body.Bytes(), payload) {
		t.Errorf("Nội dung GET không khớp với dữ liệu đã PUT")
	}

	if getRec.Header().Get("Content-Type") != "application/zstd" {
		t.Errorf("Kỳ vọng Content-Type application/zstd, nhận được: %s", getRec.Header().Get("Content-Type"))
	}
}

// 4. Kiểm tra PROPFIND (Depth: 1) sinh XML 207 Multi-Status
func TestWebDAV_PROPFIND(t *testing.T) {
	app, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass"))

	// Tạo 1 file trong thư mục
	putReq := httptest.NewRequest(http.MethodPut, "/books/tru_tien.epub", strings.NewReader("epub dummy data"))
	putReq.Header.Set("Authorization", authHeader)
	app.ServeHTTP(httptest.NewRecorder(), putReq)

	// Gửi PROPFIND Depth: 1
	propReq := httptest.NewRequest("PROPFIND", "/books", nil)
	propReq.Header.Set("Authorization", authHeader)
	propReq.Header.Set("Depth", "1")
	propRec := httptest.NewRecorder()

	app.ServeHTTP(propRec, propReq)

	if propRec.Code != 207 {
		t.Fatalf("PROPFIND kỳ vọng 207 Multi-Status, nhận được: %d", propRec.Code)
	}

	bodyStr := propRec.Body.String()
	if !strings.Contains(bodyStr, "<d:multistatus xmlns:d=\"DAV:\">") {
		t.Errorf("XML thiếu root tag multistatus")
	}
	if !strings.Contains(bodyStr, "<d:collection>") && !strings.Contains(bodyStr, "<d:collection/>") {
		t.Errorf("XML thiếu d:collection cho thư mục /books")
	}
	if !strings.Contains(bodyStr, "tru_tien.epub") {
		t.Errorf("XML thiếu tệp con tru_tien.epub")
	}
	if !strings.Contains(bodyStr, "<d:getcontentlength>") {
		t.Errorf("XML thiếu getcontentlength cho tệp")
	}
}

// 5. Kiểm tra Basic Auth chặn truy cập trái phép
func TestWebDAV_BasicAuth(t *testing.T) {
	app, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Không có header Authorization
	req1 := httptest.NewRequest(http.MethodGet, "/vBook", nil)
	rec1 := httptest.NewRecorder()
	app.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("Kỳ vọng 401 Unauthorized khi thiếu auth, nhận: %d", rec1.Code)
	}
	if !strings.Contains(rec1.Header().Get("WWW-Authenticate"), "Basic") {
		t.Errorf("Thiếu header WWW-Authenticate")
	}

	// Sai mật khẩu
	req2 := httptest.NewRequest(http.MethodGet, "/vBook", nil)
	req2.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("testuser:wrongpass")))
	rec2 := httptest.NewRecorder()
	app.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("Kỳ vọng 401 khi sai password, nhận: %d", rec2.Code)
	}
}

// 6. Kiểm tra MOVE và DELETE
func TestWebDAV_MoveAndDelete(t *testing.T) {
	app, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass"))

	// Tạo file
	putReq := httptest.NewRequest(http.MethodPut, "/file1.txt", strings.NewReader("hello"))
	putReq.Header.Set("Authorization", authHeader)
	app.ServeHTTP(httptest.NewRecorder(), putReq)

	// MOVE sang /file2.txt
	moveReq := httptest.NewRequest("MOVE", "/file1.txt", nil)
	moveReq.Header.Set("Authorization", authHeader)
	moveReq.Header.Set("Destination", "http://localhost:8080/file2.txt")
	moveRec := httptest.NewRecorder()
	app.ServeHTTP(moveRec, moveReq)

	if moveRec.Code != http.StatusCreated {
		t.Errorf("MOVE kỳ vọng 201 Created, nhận: %d", moveRec.Code)
	}

	// GET file1 (phải 404)
	get1 := httptest.NewRequest(http.MethodGet, "/file1.txt", nil)
	get1.Header.Set("Authorization", authHeader)
	recG1 := httptest.NewRecorder()
	app.ServeHTTP(recG1, get1)
	if recG1.Code != http.StatusNotFound {
		t.Errorf("file1 sau khi move phải là 404, nhận: %d", recG1.Code)
	}

	// GET file2 (phải 200)
	get2 := httptest.NewRequest(http.MethodGet, "/file2.txt", nil)
	get2.Header.Set("Authorization", authHeader)
	recG2 := httptest.NewRecorder()
	app.ServeHTTP(recG2, get2)
	if recG2.Code != http.StatusOK {
		t.Errorf("file2 sau khi move phải là 200, nhận: %d", recG2.Code)
	}

	// DELETE file2
	delReq := httptest.NewRequest(http.MethodDelete, "/file2.txt", nil)
	delReq.Header.Set("Authorization", authHeader)
	delRec := httptest.NewRecorder()
	app.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusNoContent {
		t.Errorf("DELETE kỳ vọng 204 No Content, nhận: %d", delRec.Code)
	}
}

func TestHandler_SyncHooks(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vbook_hooks_test_*")
	if err != nil {
		t.Fatalf("Failed to create tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	st, err := local.New(tempDir)
	if err != nil {
		t.Fatalf("Failed to init storage: %v", err)
	}

	handler := NewHandler(st)

	putChan := make(chan string, 1)
	delChan := make(chan string, 1)

	handler.SetSyncHooks(
		func(relPath string) { putChan <- relPath },
		func(relPath string) { delChan <- relPath },
	)

	// 1. Trigger PUT
	putReq := httptest.NewRequest(http.MethodPut, "/vBook/hook_test.txt", strings.NewReader("hello hook"))
	recPut := httptest.NewRecorder()
	handler.ServeHTTP(recPut, putReq)

	if recPut.Code != http.StatusCreated {
		t.Fatalf("PUT failed with status %d", recPut.Code)
	}

	select {
	case p := <-putChan:
		if p != "/vBook/hook_test.txt" {
			t.Errorf("Expected onPut path /vBook/hook_test.txt, got %s", p)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for onPut hook")
	}

	// 2. Trigger DELETE
	delReq := httptest.NewRequest(http.MethodDelete, "/vBook/hook_test.txt", nil)
	recDel := httptest.NewRecorder()
	handler.ServeHTTP(recDel, delReq)

	if recDel.Code != http.StatusNoContent {
		t.Fatalf("DELETE failed with status %d", recDel.Code)
	}

	select {
	case d := <-delChan:
		if d != "/vBook/hook_test.txt" {
			t.Errorf("Expected onDelete path /vBook/hook_test.txt, got %s", d)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for onDelete hook")
	}
}

func TestAuth_FormatSize(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{87 * 1048576, "87.0 MB"},
	}

	for _, tc := range tests {
		actual := formatSize(tc.input)
		if actual != tc.expected {
			t.Errorf("formatSize(%d) = %s; want %s", tc.input, actual, tc.expected)
		}
	}
}

func TestHealthzEndpoint(t *testing.T) {
	app, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /healthz expected 200 OK, got: %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "ok") {
		t.Errorf("GET /healthz expected 'ok', got: %s", body)
	}
}

func TestRootCollectionProtection(t *testing.T) {
	app, tempDir := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass"))

	// 1. DELETE / must be rejected
	reqDelete := httptest.NewRequest(http.MethodDelete, "/", nil)
	reqDelete.Header.Set("Authorization", authHeader)
	recDelete := httptest.NewRecorder()
	app.ServeHTTP(recDelete, reqDelete)

	if recDelete.Code != http.StatusForbidden {
		t.Errorf("DELETE / expected 403 Forbidden, got: %d", recDelete.Code)
	}

	// Verify tempDir was not deleted
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Fatal("FATAL: baseDir was deleted by DELETE /")
	}

	// 2. PUT / must be rejected
	reqPut := httptest.NewRequest(http.MethodPut, "/", strings.NewReader("bad data"))
	reqPut.Header.Set("Authorization", authHeader)
	recPut := httptest.NewRecorder()
	app.ServeHTTP(recPut, reqPut)

	if recPut.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT / expected 405 Method Not Allowed, got: %d", recPut.Code)
	}

	// 3. MOVE / must be rejected
	reqMove := httptest.NewRequest("MOVE", "/", nil)
	reqMove.Header.Set("Authorization", authHeader)
	reqMove.Header.Set("Destination", "/dest")
	recMove := httptest.NewRecorder()
	app.ServeHTTP(recMove, reqMove)

	if recMove.Code != http.StatusForbidden {
		t.Errorf("MOVE / expected 403 Forbidden, got: %d", recMove.Code)
	}
}
