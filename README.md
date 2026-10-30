# vbook-dav

[![Go Version](https://img.shields.io/badge/go-1.24-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Docker Image](https://img.shields.io/badge/docker-ready-2496ED?logo=docker&logoColor=white)](https://github.com/kychitoge/vbook-dav)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Footprint](https://img.shields.io/badge/footprint-%3C20MB-brightgreen.svg)](#1-giới-thiệu-tổng-quan--giá-trị-cốt-lõi)

[Tiếng Việt](#-tiếng-việt) • [English](#-english)

---

<a id="-tiếng-việt"></a>
## 🇻🇳 Tiếng Việt

Máy chủ WebDAV độc lập, hiệu năng cao, siêu nhẹ được kiến trúc chuyên biệt cho ứng dụng đọc truyện **vBook** trên Android.

`vbook-dav` mang lại trải nghiệm sao lưu tức thì qua mạng nội bộ (dưới 7 giây cho gói dữ liệu 87 MB), đồng thời hỗ trợ đồng bộ ngầm bản sao lưu lên Cloudflare R2 ($0) và gửi thông báo trạng thái qua Discord webhook.

---

### 1. Giới Thiệu Tổng Quan & Giá Trị Cốt Lõi

Các máy chủ WebDAV đa năng (như Apache, Nginx, Nextcloud hoặc rclone) thường gặp lỗi gián đoạn hoặc quá hạn thời gian kết nối (timeout) khi tương tác với vBook. `vbook-dav` được thiết kế chuyên biệt để giải quyết triệt để các vấn đề này:

- **Tương thích 100% ứng dụng vBook**: Xử lý triệt để 4 dị biệt giao thức của vBook (Idempotent `MKCOL`, Tự động tạo thư mục cha `Auto-MkdirAll`, Bypass xác thực cho `OPTIONS`, Ghi nhật ký truyền tải thời gian thực).
- **Tốc độ vượt trội (Local-first write-behind)**: Ghi trực tiếp bản sao lưu vào ổ đĩa máy chủ nội bộ và phản hồi `201 Created` ngay lập tức. Thiết bị di động không phải chờ đợi đường truyền internet quốc tế.
- **Sao lưu đám mây phụ $0**: Goroutine chạy ngầm tự động đẩy bản sao lưu lên Cloudflare R2 qua API tương thích S3 mà không làm suy giảm hiệu năng xử lý của máy chủ.
- **Cảnh báo tức thời qua Discord**: Bắn thông báo trạng thái sao lưu và cảnh báo lỗi trực tiếp về kênh Discord qua webhook bất đồng bộ, không phong tỏa luồng chính.
- **Nguyên tắc dao cạo Occam (Occam's razor)**: Vận hành trực tiếp từ một tệp nhị phân duy nhất, loại bỏ hoàn toàn cơ sở dữ liệu (không SQLite, MySQL hay Turso). Quản trị người dùng và cấu hình thông qua tệp `.env`.
- **Dung lượng siêu nhẹ**: Kích thước binary dưới 15 MB, dung lượng RAM sử dụng thực tế dưới 20 MB. Chạy ổn định trên Raspberry Pi, NAS gia đình hoặc máy tính cá nhân.

---

### 2. Giao Diện Terminal Tối Giản (Minimalist CLI)

Khi khởi động, `vbook-dav` hiển thị bảng thông số vận hành rõ ràng, tự động nhận diện IP mạng nội bộ (LAN):

```text
────────────────────────────────────────────────────────────
  vbook-dav  WebDAV Gateway v1.0
  High-performance local WebDAV backup server for vBook
────────────────────────────────────────────────────────────
  Configuration
  • Storage directory : /data
  • Authentication    : user=vbook (Read/Write)
  • Cloud replication : Active (bucket: vbook-backup)
  • Discord alerts    : Enabled

  Network endpoints
  • Wi-Fi             : http://192.168.1.100:8080/

  vBook app setup (Settings > Backup WebDAV)
  • Server URL        : http://192.168.1.100:8080/
  • Target folder     : vbook_backup
  • Username          : vbook
  • Password          : •••••••• (configured in .env)
────────────────────────────────────────────────────────────
```

---

### 3. Khởi Động Nhanh Trong 30 Giây (Quickstart)

#### Cách 1: Docker Compose (Khuyến nghị cho self-host)
```bash
git clone https://github.com/kychitoge/vbook-dav.git
cd vbook-dav
cp .env.example .env
docker compose up -d
```

#### Cách 2: File nhị phân dựng sẵn (Binary)
Tải bản binary tương ứng từ trang [Releases](https://github.com/kychitoge/vbook-dav/releases):
```bash
# Windows
Copy-Item .env.example .env
.\vbook-dav.exe

# Linux / macOS
cp .env.example .env
chmod +x ./vbook-dav
./vbook-dav
```

---

### 4. Thiết Lập Trên Ứng Dụng vBook

1. Mở ứng dụng **vBook** trên điện thoại Android.
2. Điều hướng vào menu **Cài đặt** > chọn mục **Đồng bộ & sao lưu** > chọn phương thức **WebDAV**.
3. Điền thông tin theo bảng thông số:

| Mục trên vBook | Giá trị cấu hình | Hướng dẫn chi tiết |
|---|---|---|
| **URL** | `http://<IP-MAY-CHU>:8080/` | Thay `<IP-MAY-CHU>` bằng IP LAN hiển thị trên terminal khi máy chủ khởi động. Bắt buộc có dấu `/` ở cuối. |
| **Thư mục gốc** | `vbook_backup` | Tên thư mục sao lưu mặc định. |
| **Tài khoản** | `vbook` | Giá trị biến `DAV_USER` trong `.env`. |
| **Mật khẩu** | `vbook123` | Giá trị biến `DAV_PASS` trong `.env`. |

4. Chọn **Kiểm tra kết nối** > Nhận thông báo thành công > Chọn **Sao lưu ngay**.

---

### 5. Thư Mục Tài Liệu Chuyên Sâu (Documentation Index)

Dự án tuân thủ nguyên tắc Single Responsibility Principle (SRP) cho toàn bộ tài liệu:

| Tài liệu | Trách nhiệm chuyên môn duy nhất |
|---|---|
| [**Kiến Trúc Hệ Thống (architecture.md)**](docs/architecture.md) | Thiết kế kỹ thuật, luồng dữ liệu, quy trình ghi đĩa nguyên tử và sao lưu R2. |
| [**Tra Cứu Cấu Hình (configuration.md)**](docs/configuration.md) | Toàn bộ biến môi trường, cờ CLI, thứ tự ưu tiên và quy chuẩn bảo mật. |
| [**Hướng Dẫn Tự Triển Khai (self-hosting.md)**](docs/self-hosting.md) | Triển khai Docker, Linux systemd, Tailscale VPN, Cloudflare Tunnel và Caddy HTTPS. |
| [**Đặc Tả Giao Thức vBook (vbook-protocol.md)**](docs/vbook-protocol.md) | Phân tích 4 dị biệt WebDAV của vBook, cấu trúc gói `.tar.zst` và duyệt kho sách. |
| [**Xử Lý Sự Cố (troubleshooting.md)**](docs/troubleshooting.md) | Khắc phục sự cố theo mô hình Tam Giác Lỗi (Sự việc • Nguyên nhân • Giải pháp). |

---

### 6. Đóng Góp & Bản Quyền

- Hướng dẫn đóng góp: Vui lòng xem [CONTRIBUTING.md](CONTRIBUTING.md).
- Giấy phép: Phát hành theo giấy phép mã nguồn mở [MIT License](LICENSE).

---

<a id="-english"></a>
## 🇬🇧 English

A high-performance, lightweight, self-hosted WebDAV gateway optimized specifically for the **vBook** Android reading application.

`vbook-dav` delivers instant local backups (under 7 seconds for an 87 MB library archive) while optionally replicating your backup archives to Cloudflare R2 ($0 free tier) and sending Discord status alerts in the background.

---

### 1. Overview and Core Architectural Values

Generic WebDAV servers (such as Apache, Nginx, Nextcloud, or rclone) often encounter timeouts or connection drops when interacting with vBook. `vbook-dav` is purpose-built to solve these challenges:

- **100% vBook Compatibility**: Resolves all 4 client quirks out of the box (Idempotent `MKCOL`, Parent directory `Auto-MkdirAll`, Authentication bypass for `OPTIONS`, Real-time audit telemetry).
- **Local-First Write-Behind Speed**: Commits backups to local storage immediately and returns `201 Created` in milliseconds. The mobile device does not wait for international cloud uploads.
- **Zero-Cost Cloud Replication**: An asynchronous background worker replicates backups to Cloudflare R2 using standard S3 APIs without delaying client requests.
- **Discord Webhook Alerts**: Dispatches upload/delete notifications and error alerts directly to your Discord channel through a non-blocking queue.
- **Occam's Razor Design**: Operates from a single standalone binary with zero database dependencies (no SQLite, MySQL, or Turso). Credentials and settings are managed via environment variables and `.env`.
- **Tiny Footprint**: Binary size under 15 MB and operational memory consumption under 20 MB RAM. Operates smoothly on Raspberry Pi, personal computers, and network-attached storage (NAS).

---

### 2. Minimalist Terminal Interface

Upon startup, `vbook-dav` renders a clean operational summary and automatically detects local LAN endpoints:

```text
────────────────────────────────────────────────────────────
  vbook-dav  WebDAV Gateway v1.0
  High-performance local WebDAV backup server for vBook
────────────────────────────────────────────────────────────
  Configuration
  • Storage directory : /data
  • Authentication    : user=vbook (Read/Write)
  • Cloud replication : Active (bucket: vbook-backup)
  • Discord alerts    : Enabled

  Network endpoints
  • Wi-Fi             : http://192.168.1.100:8080/

  vBook app setup (Settings > Backup WebDAV)
  • Server URL        : http://192.168.1.100:8080/
  • Target folder     : vbook_backup
  • Username          : vbook
  • Password          : •••••••• (configured in .env)
────────────────────────────────────────────────────────────
```

---

### 3. 30-Second Quickstart

#### Option 1: Docker Compose (Recommended)
```bash
git clone https://github.com/kychitoge/vbook-dav.git
cd vbook-dav
cp .env.example .env
docker compose up -d
```

#### Option 2: Pre-Built Binary
Download the matching binary from [Releases](https://github.com/kychitoge/vbook-dav/releases):
```bash
# Windows
Copy-Item .env.example .env
.\vbook-dav.exe

# Linux / macOS
cp .env.example .env
chmod +x ./vbook-dav
./vbook-dav
```

---

### 4. Setup in vBook Mobile Application

1. Open **vBook** on your Android device.
2. Navigate to **Settings** > **Backup and sync** > select **WebDAV**.
3. Configure the fields as follows:

| Field in vBook | Recommended Value | Description |
|---|---|---|
| **URL** | `http://<SERVER-IP>:8080/` | Replace `<SERVER-IP>` with the LAN IP printed in your startup banner. Must end with a trailing slash `/`. |
| **Root Path** | `vbook_backup` | Default backup directory on the server. |
| **Username** | `vbook` | Matches `DAV_USER` in `.env`. |
| **Password** | `vbook123` | Matches `DAV_PASS` in `.env`. |

4. Select **Test connection** > Confirm success > Select **Backup now**.

---

### 5. Documentation Directory

Every document in this repository adheres to the Single Responsibility Principle (SRP):

| Document | Dedicated Responsibility |
|---|---|
| [**System Architecture (architecture.md)**](docs/architecture.md) | Technical architecture, data flows, atomic write pipeline, and R2 replication. |
| [**Configuration Reference (configuration.md)**](docs/configuration.md) | Parameter reference, environment variables, CLI flags, precedence, and security. |
| [**Self-Hosting Guide (self-hosting.md)**](docs/self-hosting.md) | Docker Compose, Linux systemd, Tailscale VPN, Cloudflare Tunnel, and Caddy SSL. |
| [**vBook Protocol Specification (vbook-protocol.md)**](docs/vbook-protocol.md) | Deep dive into the 4 WebDAV quirks, `.tar.zst` payload anatomy, and catalog browsing. |
| [**Troubleshooting Guide (troubleshooting.md)**](docs/troubleshooting.md) | Blame-free diagnostic procedures following the Error Triad model. |

---

### 6. Contributing and License

- Contributing Guidelines: Please read [CONTRIBUTING.md](CONTRIBUTING.md).
- License: Open-sourced under the [MIT License](LICENSE).
