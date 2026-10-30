# Cấu Hình Máy Chủ & Tham Số Môi Trường / Server Configuration & Environment Reference

[Tiếng Việt](#-tiếng-việt) • [English](#-english)

---

<a id="-tiếng-việt"></a>
## 🇻🇳 Tiếng Việt

Tài liệu này quy định chi tiết toàn bộ các tham số cấu hình, cờ dòng lệnh (CLI flags), biến môi trường và thứ tự ưu tiên nạp dữ liệu của `vbook-dav`.

### 1. Thứ Tự Ưu Tiên Nạp Cấu Hình (Precedence Order)

Khi máy chủ khởi động, cấu hình được nạp tuần tự từ thấp lên cao. Giá trị ở mức cao hơn sẽ ghi đè giá trị ở mức thấp hơn:

```
Giá trị mặc định trong code (Defaults)
          ↓
Tệp .env tại thư mục làm việc (nếu tồn tại)
          ↓
Biến môi trường hệ thống (OS Environment Variables / Docker ENV)
          ↓
Cờ tham số dòng lệnh (CLI Flags — Mức ưu tiên cao nhất)
```

Ví dụ: Nếu trong `.env` đặt `PORT=:8080`, nhưng khi khởi chạy bạn truyền cờ `-port :9000`, máy chủ sẽ lắng nghe trên cổng `:9000`.

---

### 2. Bảng Tra Cứu Toàn Bộ Tham Số Cấu Hình

| Biến môi trường | Cờ CLI tương ứng | Kiểu dữ liệu | Giá trị mặc định | Bắt buộc | Mô tả chi tiết chức năng |
|---|---|---|---|:---:|---|
| `PORT` | `-port` | String | `:8080` | Không | Cổng TCP máy chủ lắng nghe kết nối HTTP/WebDAV (hỗ trợ cả định dạng `:8080` và `8080`). |
| `DATA_DIR` | `-data-dir` | String | `./data` | Không | Đường dẫn thư mục cục bộ dùng để lưu trữ dữ liệu sao lưu (trong Docker mount vào `/data`). |
| `DAV_BACKUP_FOLDER` | `-backup-folder` | String | `vbook_backup` | Không | Tên thư mục con lưu trữ các tệp sao lưu vBook bên trong `DATA_DIR`. Khớp với thiết lập trên ứng dụng vBook. |
| `DAV_USER` | `-user` | String | `vbook` | Không | Tên tài khoản xác thực HTTP Basic Authentication từ ứng dụng vBook. |
| `DAV_PASS` | `-pass` | String | `vbook123` | Không | Mật khẩu xác thực HTTP Basic Authentication. Hệ thống sẽ cảnh báo nếu giữ mật khẩu mặc định. |
| `READ_ONLY` | `-read-only` | Boolean | `false` | Không | Khi đặt `true`, máy chủ chặn mọi thao tác ghi/xóa (`PUT`, `DELETE`, `MKCOL`, `MOVE`), chỉ cho phép tải dữ liệu (`GET`, `PROPFIND`, `HEAD`). |
| `R2_ENDPOINT` | `-r2-endpoint` | String | *(trống)* | Không | Đường dẫn S3 API endpoint do Cloudflare R2 cấp (ví dụ: `https://<account_id>.r2.cloudflarestorage.com`). |
| `R2_ACCESS_KEY` | `-r2-access-key` | String | *(trống)* | Không | Mã định danh Access Key ID của Cloudflare R2. |
| `R2_SECRET_KEY` | `-r2-secret-key` | String | *(trống)* | Không | Mã khóa bí mật Secret Access Key của Cloudflare R2. |
| `R2_BUCKET` | `-r2-bucket` | String | *(trống)* | Không | Tên thùng chứa (Bucket) lưu trữ bản sao lưu trên Cloudflare R2. |
| `DISCORD_WEBHOOK_URL` | `-discord-webhook` | String | *(trống)* | Không | Đường dẫn Webhook của kênh Discord để nhận thông báo đồng bộ và cảnh báo lỗi. |

> [!NOTE]
> - Nếu một trong bốn biến `R2_*` bị bỏ trống, tính năng đồng bộ đám mây sẽ tự động tắt, máy chủ vận hành an toàn ở chế độ thuần cục bộ (local-only mode).
> - Nếu `DISCORD_WEBHOOK_URL` bị bỏ trống, hệ thống sử dụng cơ chế Null Object (noop), không phát sinh cuộc gọi mạng ra ngoài.

---

### 3. Quy Tắc Nạp Tệp `.env`

Bộ nạp cấu hình tích hợp sẵn tự động đọc tệp `.env` tại thư mục hiện hành:
- **Dòng chú thích**: Bắt đầu bằng dấu thăng `#` và sẽ được bỏ qua.
- **Dòng trống**: Được tự động bỏ qua.
- **Cặp khóa - giá trị**: Phân tách bằng dấu `=`, định dạng `KEY=VALUE`.
- **Dấu ngoặc kép hoặc đơn**: Tự động lược bỏ nếu giá trị được bao bọc bởi `"` hoặc `'` (ví dụ: `DAV_PASS="my_secret"` $\rightarrow$ `my_secret`).
- **Khoảng trắng thừa**: Tự động cắt tỉa (`TrimSpace`) ở cả khóa và giá trị.
- **Không ghi đè biến hệ thống**: Nếu biến môi trường hệ điều hành đã được thiết lập từ trước, trình nạp `.env` sẽ giữ nguyên biến môi trường đó và không ghi đè.

---

### 4. Khuyến Nghị An Toàn Bảo Mật & Docker Mapping

1. **Thay đổi mật khẩu mặc định**: Luôn đổi `DAV_PASS` khác `vbook123` trước khi triển khai môi trường sản xuất.
2. **Bảo vệ tệp `.env`**: Thiết lập quyền truy cập nghiêm ngặt trên Linux/macOS:
   ```bash
   chmod 600 .env
   ```
3. **Không commit `.env` lên Git**: Tệp `.gitignore` mặc định đã chặn tệp `.env`. Chỉ chia sẻ tệp mẫu `.env.example`.
4. **Chế độ chỉ đọc (Read-Only)**: Kích hoạt `READ_ONLY=true` khi bạn dùng máy chủ này làm kho đọc truyện ngoại tuyến (`Cloud Storage`) để tránh việc ứng dụng vô tình ghi đè hoặc xóa sách.
5. **Ánh xạ cổng trong Docker**: File `docker-compose.yml` mặc định ánh xạ cổng `"8080:8080"`. Nếu bạn muốn đổi cổng bên ngoài máy chủ host (ví dụ cổng 9000), sửa dòng `ports` thành `"- 9000:8080"` trong `docker-compose.yml` thay vì đổi biến `PORT` bên trong container.

---

<a id="-english"></a>
## 🇬🇧 English

This document provides a comprehensive reference for all configuration parameters, command-line flags, environment variables, and loading precedence in `vbook-dav`.

### 1. Configuration Precedence

When the server initializes, configuration values are resolved in ascending order of priority. Higher levels override lower levels:

```
Built-in code defaults
          ↓
.env file in working directory (if present)
          ↓
Operating system environment variables (Docker ENV)
          ↓
Command-line flags (CLI Flags — Highest precedence)
```

Example: If `.env` defines `PORT=:8080` but you start the server with `-port :9000`, the server listens on port `:9000`.

---

### 2. Parameter Reference Table

| Environment Variable | CLI Flag | Data Type | Default Value | Required | Description |
|---|---|---|---|:---:|---|
| `PORT` | `-port` | String | `:8080` | No | TCP listening port for HTTP/WebDAV (supports both `:8080` and `8080`). |
| `DATA_DIR` | `-data-dir` | String | `./data` | No | Local filesystem path where files and backups are stored (mounted to `/data` in Docker). |
| `DAV_BACKUP_FOLDER` | `-backup-folder` | String | `vbook_backup` | No | Subdirectory name within `DATA_DIR` designated for vBook backups. Must match the app settings. |
| `DAV_USER` | `-user` | String | `vbook` | No | HTTP Basic Authentication username expected from the vBook client. |
| `DAV_PASS` | `-pass` | String | `vbook123` | No | HTTP Basic Authentication password. The server logs a warning if default is kept. |
| `READ_ONLY` | `-read-only` | Boolean | `false` | No | When set to `true`, mutating operations (`PUT`, `DELETE`, `MKCOL`, `MOVE`) are rejected with `403 Forbidden`. Only read operations are permitted. |
| `R2_ENDPOINT` | `-r2-endpoint` | String | *(empty)* | No | Cloudflare R2 S3 API endpoint URL (e.g., `https://<account_id>.r2.cloudflarestorage.com`). |
| `R2_ACCESS_KEY` | `-r2-access-key` | String | *(empty)* | No | Cloudflare R2 Access Key ID. |
| `R2_SECRET_KEY` | `-r2-secret-key` | String | *(empty)* | No | Cloudflare R2 Secret Access Key. |
| `R2_BUCKET` | `-r2-bucket` | String | *(empty)* | No | Destination Cloudflare R2 bucket name. |
| `DISCORD_WEBHOOK_URL` | `-discord-webhook` | String | *(empty)* | No | Discord incoming webhook URL for sync notifications and error alerts. |

> [!NOTE]
> - If any of the four `R2_*` variables are left empty, cloud replication is disabled, and the server operates securely in local-only mode.
> - If `DISCORD_WEBHOOK_URL` is empty, notifications are disabled using a safe Null Object pattern with zero external network overhead.

---

### 3. `.env` Parsing Specification

The built-in loader parses `.env` files with the following rules:
- **Comment lines**: Lines beginning with `#` are ignored.
- **Blank lines**: Ignored automatically.
- **Key-value pairs**: Separated by `=`, formatted as `KEY=VALUE`.
- **Quote stripping**: Enclosing single (`'`) or double (`"`) quotes are stripped automatically.
- **Whitespace trimming**: Leading and trailing whitespace is trimmed from keys and values.
- **Preservation of existing environment**: If an OS environment variable already exists, the `.env` value does not overwrite it.

---

### 4. Security Best Practices & Docker Mapping

1. **Change default credentials**: Always replace `vbook123` with a strong password before production deployment.
2. **Restrict file permissions**: On Linux and macOS systems, protect the configuration file:
   ```bash
   chmod 600 .env
   ```
3. **Prevent version control leakage**: `.env` is blocked by `.gitignore`. Only commit `.env.example`.
4. **Read-only protection**: Enable `READ_ONLY=true` when utilizing the server solely as an offline reader catalog to prevent accidental deletion.
5. **Docker port mapping**: `docker-compose.yml` maps `"8080:8080"` by default. If you wish to use a different external host port (for example, 9000), update the `ports` entry to `"- 9000:8080"` in `docker-compose.yml` rather than modifying `PORT` inside the container.
