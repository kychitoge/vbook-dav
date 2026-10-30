# Đặc Tả Giao Thức WebDAV Cho Ứng Dụng vBook / vBook WebDAV Protocol Specification

[Tiếng Việt](#-tiếng-việt) • [English](#-english)

---

<a id="-tiếng-việt"></a>
## 🇻🇳 Tiếng Việt

Tài liệu này quy định chi tiết cấu trúc dữ liệu, giao thức mạng và quy chuẩn giao tiếp **WebDAV (RFC 4918 / RFC 2518)** được thiết kế tương thích 100% với ứng dụng **vBook** trên Android.

---

### 1. Bốn Dị Biệt Giao Thức (Quirks) Của vBook & Phương Án Xử Lý

Các máy chủ WebDAV tiêu chuẩn (Apache, Nginx, Nextcloud, rclone) thường gặp lỗi gián đoạn hoặc timeout khi giao tiếp với vBook do ứng dụng này có những hành vi đặc thù:

| Dị biệt | Hành vi của vBook Client | Phản hồi RFC 4918 chuẩn | Giải pháp của vbook-dav |
|---|---|---|---|
| **1. Idempotent MKCOL** | Gửi lệnh `MKCOL` ngay cả khi thư mục đích đã tồn tại sẵn trên máy chủ. | Máy chủ trả về `405 Method Not Allowed`, khiến vBook hủy tiến trình đồng bộ. | Trả về ngay `201 Created` nếu thư mục đã tồn tại, giúp client tiếp tục đồng bộ an toàn. |
| **2. Tự động tạo thư mục (Auto-MkdirAll)** | Gửi trực tiếp lệnh `PUT /vbook_backup/file.tar.zst` mà không gọi `MKCOL` trước. | Máy chủ trả về `409 Conflict` do chưa có thư mục cha. | Tự động tạo toàn bộ cây thư mục cha trước khi ghi luồng dữ liệu. |
| **3. Bypass xác thực cho OPTIONS** | Gửi yêu cầu `OPTIONS /` không đính kèm Basic Auth khi người dùng ấn "Kiểm tra kết nối". | Máy chủ yêu cầu xác thực (`401 Unauthorized`), khiến test báo thất bại. | Bỏ qua xác thực riêng cho phương thức `OPTIONS`, phản hồi ngay `200 OK` kèm header `DAV: 1, 2`. |
| **4. Đo kiểm truyền tải tệp nén** | Truyền tải các gói dữ liệu `.tar.zst` dung lượng lớn (10 MB - 100 MB+). | Máy chủ ghi log bình thường, không thống kê tốc độ. | Middleware tính toán lưu lượng bytes, tốc độ truyền tải (MB/s) và độ trễ theo thời gian thực. |

---

### 2. Phân Hệ Sao Lưu & Đồng Bộ (Backup & Sync)

#### 2.1. Cấu hình tham số kết nối từ vBook Client
Khi cấu hình trong vBook (**Cài đặt** > **Đồng bộ & sao lưu** > **WebDAV**), client sử dụng 4 tham số:
- **`URL` (`baseUrl`)**: Địa chỉ máy chủ WebDAV, bắt buộc có dấu gạch chéo cuối (ví dụ: `http://192.168.1.100:8080/`).
- **`Thư mục gốc` (`rootPath`)**: Tên thư mục sao lưu (mặc định: `vbook_backup`). Client tự động chuẩn hóa dấu gạch chéo ngược `\` thành `/`, tự động loại bỏ khoảng trắng và gán dấu `/` đầu.
- **`Tài khoản` (`userName`)**: Khớp với biến `DAV_USER` (mặc định: `vbook`).
- **`Mật khẩu` (`password`)**: Khớp với biến `DAV_PASS` (mặc định: `vbook123`).

#### 2.2. Trình tự bắt tay mạng (Network Handshake Sequence)
Khi người dùng thực hiện thao tác kiểm tra hoặc sao lưu:

```
vBook Client                             vbook-dav Gateway
     │                                           │
     │ 1. OPTIONS / (Unauthenticated)           │
     ├──────────────────────────────────────────>│
     │ <── 200 OK (DAV: 1, 2)                   │
     │                                           │
     │ 2. HEAD /vbook_backup (Basic Auth)        │
     ├──────────────────────────────────────────>│
     │ <── 200 OK hoặc 404 Not Found             │
     │                                           │
     │ 3. MKCOL /vbook_backup (nếu 404)          │
     ├──────────────────────────────────────────>│
     │ <── 201 Created (hoặc 201 nếu đã có)      │
     │                                           │
     │ 4. PUT /vbook_backup/backup.tar.zst       │
     ├──────────────────────────────────────────>│
     │ <── 201 Created (kèm ETag)                │
```

#### 2.3. Quy chuẩn đặt tên tệp sao lưu
- **Mặc định tự động**: `backup.tar.zst` (Kho lưu trữ TAR nén bằng thuật toán Zstandard - ZSTD mang lại tốc độ cực nhanh và tỷ lệ nén cao).
- **Dự phòng (Fallback)**: `backup.zip` (Được kiểm tra khi khôi phục nếu không tìm thấy tệp `.zst`).
- **Xuất thủ công**: `backup_yyyyMMdd_HHmmss.tar` (Gói sao lưu theo mốc thời gian khi người dùng chủ động xuất).

#### 2.4. Cấu trúc nội bộ của gói sao lưu `backup.tar.zst`
```text
backup.tar.zst
├── book.json              # Danh mục sách và trạng thái kệ sách
├── bookmarks.json         # Danh sách dấu trang, highlight và ghi chú đọc
├── chapters.json          # Mục lục chi tiết chương và tiến độ đọc
├── contents.json          # Siêu dữ liệu bộ nhớ đệm chương đọc ngoại tuyến
├── contents/              # Thư mục chứa nội dung văn bản các chương đã tải
├── cover/                 # Thư mục lưu ảnh bìa sách
├── extensions.json        # Danh sách tiện ích mở rộng và nguồn truyện (Extensions)
├── extension_storage.json # Cấu hình và dữ liệu lưu trữ của từng extension
└── settings.json          # Toàn bộ cài đặt giao diện, font, từ điển và cử chỉ đọc
```

#### 2.5. Luồng truyền tải nhị phân (Streaming Pipeline)
- **Tải lên (`PUT`)**: Luồng nhị phân được truyền tải theo khối đệm chuẩn **8192 bytes (8 KB)**. Máy chủ ghi tạm ra `.tmp`, gọi `Sync()` xuống đĩa, đóng handle và `Rename()` nguyên tử.
- **Tải về (`GET`)**: Máy chủ truyền phát luồng nhị phân kèm đầy đủ tiêu đề HTTP `Content-Length`, `Content-Type: application/zstd` và `ETag`.

---

### 3. Phân Hệ Kho Lưu Trữ Đám Mây & Tiện Ích (Cloud Storage / OPDS)

Hỗ trợ duyệt danh mục tệp và tải sách trực tiếp từ máy chủ cá nhân thông qua phương thức `PROPFIND`:

#### Yêu cầu truy vấn danh mục (`PROPFIND` Depth 1):
```http
PROPFIND /vbook_backup HTTP/1.1
Host: 192.168.1.100:8080
Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ=
Depth: 1
Content-Type: application/xml; charset=utf-8
```

#### Phản hồi chuẩn `207 Multi-Status`:
Máy chủ phản hồi XML chuẩn RFC 4918 biểu diễn danh sách tệp, kích thước, thời gian sửa đổi và định dạng MIME:
```xml
<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/vbook_backup/</d:href>
    <d:propstat>
      <d:status>HTTP/1.1 200 OK</d:status>
      <d:prop>
        <d:displayname>vbook_backup</d:displayname>
        <d:resourcetype><d:collection/></d:resourcetype>
        <d:getlastmodified>Fri, 30 Oct 2026 00:00:00 GMT</d:getlastmodified>
        <d:getcontenttype>httpd/unix-directory</d:getcontenttype>
      </d:prop>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/vbook_backup/backup.tar.zst</d:href>
    <d:propstat>
      <d:status>HTTP/1.1 200 OK</d:status>
      <d:prop>
        <d:displayname>backup.tar.zst</d:displayname>
        <d:getcontentlength>91226112</d:getcontentlength>
        <d:getlastmodified>Fri, 30 Oct 2026 00:00:00 GMT</d:getlastmodified>
        <d:getcontenttype>application/zstd</d:getcontenttype>
        <d:getetag>"1a2b3c-4d5e"</d:getetag>
      </d:prop>
    </d:propstat>
  </d:response>
</d:multistatus>
```

---

<a id="-english"></a>
## 🇬🇧 English

This document defines the network protocol standards, data payloads, and **WebDAV (RFC 4918 / RFC 2518)** specifications engineered for 100% compatibility with the **vBook** Android novel reader.

---

### 1. Four vBook Protocol Quirks and Server Solutions

Generic WebDAV servers (Apache, Nginx, Nextcloud, rclone) often encounter timeouts or connection drops when interacting with vBook due to four specific application behaviors:

| Quirk | vBook Client Behavior | Standard RFC 4918 Response | vbook-dav Solution |
|---|---|---|---|
| **1. Idempotent MKCOL** | Sends `MKCOL` requests even if the target directory already exists. | Server returns `405 Method Not Allowed`, aborting sync. | Immediately returns `201 Created` if the directory exists, allowing sync to proceed safely. |
| **2. Auto-MkdirAll** | Sends `PUT /vbook_backup/file.tar.zst` without a prior `MKCOL` call. | Server returns `409 Conflict` because parent directories are missing. | Automatically creates all missing ancestor directories prior to writing the file stream. |
| **3. OPTIONS Auth Bypass** | Sends unauthenticated `OPTIONS /` when selecting "Test connection". | Server challenges with `401 Unauthorized`, causing the connection test to fail. | Bypasses authentication for `OPTIONS`, returning `200 OK` with `DAV: 1, 2` headers. |
| **4. Compressed Archive Telemetry** | Transmits multi-megabyte `.tar.zst` archives (10 MB to 100 MB+). | Generic logging with no throughput metrics. | Auditing middleware computes byte counts, transfer speed (MB/s), and latency in real time. |

---

### 2. Backup and Synchronization Subsystem

#### 2.1. Client Connection Parameters
When configuring vBook (**Settings** > **Backup and sync** > **WebDAV**), the client specifies 4 parameters:
- **`URL` (`baseUrl`)**: WebDAV server endpoint, must end with a trailing slash (e.g., `http://192.168.1.100:8080/`).
- **`Root Path` (`rootPath`)**: Backup subdirectory name (default: `vbook_backup`). The client normalizes backslashes `\` to `/`, trims whitespace, and ensures a leading `/`.
- **`Username` (`userName`)**: Matches `DAV_USER` in `.env` (default: `vbook`).
- **`Password` (`password`)**: Matches `DAV_PASS` in `.env` (default: `vbook123`).

#### 2.2. Network Handshake Sequence
When a user initiates connection testing or backup synchronization:

```
vBook Client                             vbook-dav Gateway
     │                                           │
     │ 1. OPTIONS / (Unauthenticated)           │
     ├──────────────────────────────────────────>│
     │ <── 200 OK (DAV: 1, 2)                   │
     │                                           │
     │ 2. HEAD /vbook_backup (Basic Auth)        │
     ├──────────────────────────────────────────>│
     │ <── 200 OK or 404 Not Found               │
     │                                           │
     │ 3. MKCOL /vbook_backup (if 404)           │
     ├──────────────────────────────────────────>│
     │ <── 201 Created (or 201 if exists)        │
     │                                           │
     │ 4. PUT /vbook_backup/backup.tar.zst       │
     ├──────────────────────────────────────────>│
     │ <── 201 Created (with ETag)               │
```

#### 2.3. Archive Naming Conventions
- **Automatic Default**: `backup.tar.zst` (Zstandard compressed TAR archive offering ultra-fast compression and high ratio).
- **Fallback**: `backup.zip` (Checked during restore if `.zst` is not present).
- **Manual Export**: `backup_yyyyMMdd_HHmmss.tar` (Timestamped snapshot when exported manually).

#### 2.4. Internal Layout of `backup.tar.zst`
```text
backup.tar.zst
├── book.json              # Bookshelf entries and book metadata
├── bookmarks.json         # Reading bookmarks, highlights, and annotations
├── chapters.json          # Chapter catalog and reading progress
├── contents.json          # Offline chapter cache metadata
├── contents/              # Cached chapter text files
├── cover/                 # Book cover image files
├── extensions.json        # Extension sources and plugins
├── extension_storage.json # Key-value storage for extensions
└── settings.json          # Reader settings, themes, fonts, gestures
```

#### 2.5. Binary Streaming Pipeline
- **Upload (`PUT`)**: Streamed through a standard **8192-byte (8 KB)** chunk buffer. The server stages into `.tmp`, calls `Sync()`, closes the handle, and atomically renames.
- **Download (`GET`)**: Streamed with full HTTP headers including `Content-Length`, `Content-Type: application/zstd`, and `ETag`.

---

### 3. Cloud Storage Subsystem (E-Book Directory Browsing)

Supports directory navigation and book downloads via standard WebDAV `PROPFIND` methods:

#### Directory Query (`PROPFIND` Depth 1):
```http
PROPFIND /vbook_backup HTTP/1.1
Host: 192.168.1.100:8080
Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ=
Depth: 1
Content-Type: application/xml; charset=utf-8
```

#### Response (`207 Multi-Status`):
The server formats a standard RFC 4918 XML response enumerating collection items, file sizes, timestamps, and MIME types.
