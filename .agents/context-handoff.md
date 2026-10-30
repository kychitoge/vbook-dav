# Bối Cảnh Bàn Giao Tác Tử (Agent Context Handoff)

> **Dự án:** `vbook-dav` — Máy chủ WebDAV Gateway độc lập, hiệu năng cao, siêu nhẹ cho ứng dụng đọc truyện vBook trên Android  
> **Repository:** `https://github.com/kychitoge/vbook-dav`  
> **Ngôn ngữ & Runtime:** Go 1.24+ (Zero external database, zero-dependency WebDAV core)  
> **Cập nhật:** 2026-10-30  
> **Trạng thái:** **Sẵn sàng Open-Source (Production Ready - Release v1.0.0)**  
> **Mục đích tài liệu:** Cung cấp toàn bộ bối cảnh kỹ thuật, triết lý kiến trúc, quy chuẩn mã nguồn và các ràng buộc bất biến để **bất kỳ AI Agent nào (khi người dùng clone về máy cá nhân)** đều có thể nắm bắt và tiếp tục phát triển dự án chuẩn xác 100%, không bị ảo giác hay phá vỡ kiến trúc.

---

## 1. Nghiệp Vụ Cốt Lõi: vBook Là Gì & Vì Sao Cần vbook-dav?

### 1.1. Ứng dụng vBook trên Android
**vBook** là ứng dụng đọc truyện phổ biến trên Android (hỗ trợ đọc online qua extensions/nguồn truyện, tải offline, đổi font, giao diện, đánh dấu trang).
- Ứng dụng hỗ trợ đồng bộ dữ liệu người dùng (kệ sách, lịch sử đọc, chương đã tải, extensions) lên máy chủ cá nhân thông qua giao thức **WebDAV (RFC 4918 / RFC 2518)**.
- Dữ liệu sao lưu được đóng gói thành tệp nén TAR nén bằng thuật toán Zstandard: **`backup.tar.zst`** (dung lượng từ 10 MB đến hơn 100 MB).

### 1.2. Tại sao các máy chủ WebDAV thông thường (Apache, Nginx, Nextcloud, Rclone) đều thất bại?
Khi kết nối với vBook, các máy chủ WebDAV tiêu chuẩn thường gặp lỗi gián đoạn tiến trình đồng bộ do vBook có **4 hành vi giao thức đặc thù (Quirks)**:
1. **Idempotent MKCOL:** vBook gửi lệnh `MKCOL` tạo thư mục ngay cả khi thư mục đích đã tồn tại trên máy chủ. Máy chủ WebDAV chuẩn trả về `405 Method Not Allowed`, khiến vBook hủy phiên sao lưu. `vbook-dav` xử lý bằng cách trả về ngay `201 Created` nếu thư mục đã tồn tại.
2. **Auto-MkdirAll trên PUT:** vBook gửi trực tiếp `PUT /vbook_backup/file.tar.zst` mà không gọi `MKCOL` trước. Máy chủ chuẩn trả về `409 Conflict`. `vbook-dav` tự động tạo toàn bộ cây thư mục cha (`MkdirAll`) trước khi ghi file.
3. **Bypass xác thực cho OPTIONS:** Khi người dùng ấn nút "Kiểm tra kết nối" trong vBook, ứng dụng gửi request `OPTIONS /` không đính kèm header Basic Auth. Máy chủ chuẩn phản hồi `401 Unauthorized`, làm test kết nối báo thất bại. `vbook-dav` cho phép bypass xác thực riêng cho `OPTIONS`, lập tức trả về `200 OK` kèm header `DAV: 1, 2`.
4. **Đo kiểm truyền tải tệp nén lớn thời gian thực:** Các gói nén `.tar.zst` có dung lượng nhiều chục MB. Lớp middleware của `vbook-dav` tính toán và ghi log trực tiếp lưu lượng bytes, tốc độ (MB/s) và độ trễ theo thời gian thực.

---

## 2. Bốn Trụ Cột Kiến Trúc Bất Biến (Non-Negotiable Architecture)

Bất kỳ thay đổi mã nguồn nào trong tương lai **bắt buộc phải tuân thủ nghiêm ngặt 4 nguyên tắc sau**:

1. **Local-first write-behind ($0 Cloud Cost):**
   - Thiết bị di động ghi gói sao lưu trực tiếp vào ổ SSD máy chủ nội bộ qua mạng LAN (đạt tốc độ ~13.7 MB/s, xong 87 MB trong 6.4 giây).
   - Máy chủ phản hồi `201 Created` ngay lập tức cho điện thoại.
   - Một background goroutine độc lập tuần tự lấy dữ liệu từ hàng đợi đệm (`chan Job`, `cap=64`) để đẩy ngầm lên Cloudflare R2 qua API tương thích S3 và gửi thông báo Discord webhook. Nếu R2 hoặc Discord không cấu hình, máy chủ chạy thuần local an toàn (Graceful Fallback), không bao giờ crash.
2. **Dao cạo Occam (Tuyệt đối không sử dụng cơ sở dữ liệu):**
   - Không đưa thêm SQLite, PostgreSQL, MySQL hay Turso vào dự án.
   - Quản trị tài khoản duy nhất qua biến môi trường (`DAV_USER`, `DAV_PASS`). So khớp mật khẩu bằng `subtle.ConstantTimeCompare` để chống tấn công Timing Attack.
   - Không có cơ sở dữ liệu đồng nghĩa với: không lỗi khóa file (database locked), không lỗi migration, không gánh nặng bảo trì.
3. **Quy trình ghi tệp nguyên tử (Atomic Write Pipeline):**
   - Dữ liệu tải lên được truyền qua buffer chuẩn 8192 bytes vào tệp tạm `<target>.<uuid>.tmp`.
   - Gọi `file.Sync()` ép hệ điều hành ghi xuống đĩa vật lý.
   - Đóng file handle trước khi đổi tên (Bắt buộc trên Windows để tránh lỗi `ERROR_SHARING_VIOLATION`).
   - Gọi `os.Rename()` thay thế tệp đích tức thời và nguyên tử. Không bao giờ để xảy ra tình trạng tệp sao lưu bị ghi dở hoặc hỏng hóc.
   - Khi khởi động, cơ chế `CleanupTempFiles()` tự động dọn sạch các file `.tmp` cũ từ phiên bị gián đoạn.
4. **Bảo vệ biên lưu trữ & Thư mục gốc:**
   - Chống Path Traversal: Chuẩn hóa bằng `path.Clean` và kiểm tra `filepath.Rel` với `baseDir`.
   - Chặn luồng dữ liệu phụ Windows NTFS (`:`).
   - Chặn tên thiết bị cấm DOS trên Windows (`CON`, `PRN`, `AUX`, `NUL`, `COM1-9`, `LPT1-9`).
   - Chặn tuyệt đối thao tác nguy hiểm `DELETE /`, `PUT /`, `MOVE /` nhắm vào thư mục gốc (`403 Forbidden` / `405 Method Not Allowed`).

---

## 3. Bản Đồ Cấu Trúc Mã Nguồn Dự Án

```text
vbook-dav/
├── .agents/
│   ├── context-handoff.md              # [Tài liệu này] Bối cảnh bàn giao toàn diện cho mọi Agent
│   └── decisions.md                    # Sổ ghi nhận quyết định kiến trúc ADR-001 đến ADR-006
├── .github/
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.md               # Mẫu báo cáo sự cố chuẩn Tam Giác Lỗi Microsoft
│   │   └── feature_request.md          # Mẫu đề xuất tính năng kèm checklist Dao cạo Occam
│   ├── workflows/
│   │   ├── ci.yml                      # CI kiểm thử tự động (vet, test, build) trên Ubuntu runner
│   │   └── release.yml                 # Workflow đóng gói tự động cho 4 OS phổ biến khi push tag v*
│   └── pull_request_template.md        # Mẫu Pull Request kèm checklist thẩm định kỹ thuật
├── cmd/dav/
│   ├── console_windows.go              # Kích hoạt UTF-8 CP 65001 & Virtual Terminal trên Windows
│   ├── console_other.go                # Stub cho Linux/macOS
│   └── main.go                         # Entry point: nạp config, storage, syncer, notifier, banner Zinc, graceful shutdown
├── data/
│   └── .gitkeep                        # Giữ thư mục data trên Git, toàn bộ file sao lưu *.tar.zst bị gitignore
├── docs/                               # Bộ tài liệu chuẩn Single Responsibility Principle (Song ngữ 🇻🇳/🇬🇧)
│   ├── architecture.md                 # [SRP 1] Sơ đồ kiến trúc, luồng dữ liệu, atomic writes
│   ├── configuration.md                # [SRP 2] Bảng từ điển 11 biến môi trường, flags, precedence, Docker
│   ├── self-hosting.md                 # [SRP 3] Cẩm nang deploy Docker Compose, systemd, Tailscale, Cloudflare
│   ├── troubleshooting.md              # [SRP 4] Chẩn đoán theo Tam Giác Lỗi Microsoft (Sự việc • Nguyên nhân • Giải pháp)
│   └── vbook-protocol.md               # [SRP 5] Đặc tả kỹ thuật giao thức vBook, handshake HTTP, giải phẫu .tar.zst
├── internal/
│   ├── config/
│   │   ├── config.go                   # Nạp .env, CLI flags, chuẩn hóa PORT, DATA_DIR, R2, Discord
│   │   └── config_test.go              # Unit test nạp config, kiểm tra thứ tự ưu tiên flags
│   ├── notify/
│   │   ├── notify.go                   # Notifier interface, Discord webhook, Null Object pattern
│   │   ├── notify_test.go              # Unit test gửi webhook Discord
│   │   └── adversarial_test.go         # Test chịu lỗi: webhook timeout, URL hỏng, payload dị dạng
│   ├── r2sync/
│   │   ├── r2sync.go                   # Client AWS S3 cho R2, channel queue cap=64, startup scan đẩy bù
│   │   ├── r2sync_test.go              # Unit test worker queue, upload, delete
│   │   └── adversarial_test.go         # Test nghẽn hàng đợi, mất mạng, context cancellation
│   ├── storage/
│   │   ├── engine.go                   # StorageEngine interface & FileItem model
│   │   └── local/
│   │       ├── local.go                # Ghi đĩa cục bộ, buffer 8KB, atomic rename, path traversal defense
│   │       ├── local_test.go           # Unit test CRUD, streaming, cleanup temp files
│   │       └── local_adversarial_test.go # Test path traversal (NTFS ADS, Windows DOS reserved names)
│   └── webdav/
│       ├── auth.go                     # Basic Auth constant-time, probe /healthz, audit log Zinc minimalist
│       ├── handler.go                  # WebDAV method dispatcher (4 dị biệt vBook, sync hooks)
│       ├── handler_test.go             # Unit test 4 dị biệt, root protection, CRUD
│       ├── handler_adversarial_test.go # Test request dị dạng, concurrent write
│       └── xml.go                      # 207 Multi-Status XML generator chuẩn RFC 4918
├── scripts/
│   └── build-release.ps1               # Script PowerShell build cục bộ 4 OS phổ biến nhất trong 30s
├── .dockerignore                       # Loại trừ .env, data, bin, git khỏi build context
├── .env                                # File cấu hình môi trường thực tế (Được .gitignore bảo vệ tuyệt đối)
├── .env.example                        # File mẫu cấu hình môi trường chuẩn cho người dùng
├── .gitignore                          # Chặn .env, data/*, *.exe, dist/, *.tmp, .agents/teamwork/
├── CONTRIBUTING.md                     # Hướng dẫn đóng góp mã nguồn (Song ngữ, chuẩn Microsoft Style Guide)
├── docker-compose.yml                  # Docker Compose production-ready (UID 1000, probe /healthz)
├── Dockerfile                          # Multi-stage build (Alpine 3.21, non-root user, static binary)
├── go.mod                              # Go 1.24, AWS SDK Go v2
├── go.sum                              # Checksum phụ thuộc
├── LICENSE                             # Giấy phép mã nguồn mở MIT
└── README.md                           # Cổng thông tin dự án chính (Song ngữ 🇻🇳/🇬🇧)
```

---

## 4. Bảng Tham Số Cấu Hình Hệ Thống (Configuration Dictionary)

Hệ thống hỗ trợ 11 tham số cấu hình. Thứ tự ưu tiên:
$$\text{Code Defaults} \longrightarrow \text{.env File} \longrightarrow \text{OS / Docker ENV} \longrightarrow \text{CLI Flags (Cao nhất)}$$

| Biến môi trường | Cờ CLI | Mặc định | Ý nghĩa chức năng |
|---|---|---|---|
| `PORT` | `-port` | `:8080` | Cổng TCP lắng nghe (hỗ trợ cả `:8080` và `8080`). |
| `DATA_DIR` | `-data-dir` | `./data` | Thư mục lưu trữ tệp sao lưu trên máy chủ (trong Docker là `/data`). |
| `DAV_BACKUP_FOLDER` | `-backup-folder` | `vbook_backup` | Thư mục con cho các bản sao lưu vBook. |
| `DAV_USER` | `-user` | `vbook` | Tên đăng nhập Basic Auth. |
| `DAV_PASS` | `-pass` | `vbook123` | Mật khẩu Basic Auth (hệ thống cảnh báo nếu giữ mặc định). |
| `READ_ONLY` | `-read-only` | `false` | Chế độ chỉ đọc. Chặn `PUT`, `DELETE`, `MKCOL`, `MOVE`. |
| `R2_ENDPOINT` | `-r2-endpoint` | *(trống)* | URL S3 endpoint của Cloudflare R2 (ví dụ: `https://<id>.r2.cloudflarestorage.com`). |
| `R2_ACCESS_KEY` | `-r2-access-key` | *(trống)* | R2 Access Key ID. |
| `R2_SECRET_KEY` | `-r2-secret-key` | *(trống)* | R2 Secret Access Key. |
| `R2_BUCKET` | `-r2-bucket` | *(trống)* | Tên thùng chứa R2. |
| `DISCORD_WEBHOOK_URL` | `-discord-webhook` | *(trống)* | URL webhook Discord nhận thông báo sao lưu và lỗi. |

---

## 5. Quy Chuẩn Kiểm Thử & Lệnh Điều Hành

> [!IMPORTANT]
> **Cảnh báo Paging File trên Windows:** Khi chạy kiểm thử trên máy chủ Windows, Go test có thể gặp lỗi cạn kiệt paging memory nếu chạy song song. **Bắt buộc** truyền cờ `-p 1` để chạy tuần tự các package.

```powershell
# 1. Chạy toàn bộ test suite (Unit & Adversarial)
go test -count=1 -p 1 ./internal/config ./internal/storage/local ./internal/webdav ./internal/r2sync ./internal/notify

# 2. Phân tích tĩnh (Static Analysis)
go vet ./...

# 3. Định dạng mã nguồn chuẩn Go
go fmt ./...

# 4. Kiểm tra biên dịch nhị phân
go build ./cmd/dav

# 5. Đóng gói bản phát hành cục bộ đa nền tảng (30 giây)
.\scripts\build-release.ps1
```

---

## 6. Quy Trình Phát Hành Đa Nền Tảng (Cross-Platform Releases)

Dự án hỗ trợ 4 hệ điều hành phổ biến nhất:
1. **Linux x86_64** (`linux_amd64`): VPS Ubuntu/Debian, Homelab x86.
2. **Linux ARM64** (`linux_arm64`): Raspberry Pi 4/5, server Cloud ARM.
3. **Windows x64** (`windows_amd64`): Máy tính cá nhân Windows 10/11.
4. **macOS Apple Silicon** (`darwin_arm64`): Mac chip M1/M2/M3/M4.

**Cách phát hành:**
- *Cách 1 (Khuyên dùng - 100% không lỗi vặt):* Chạy `.\scripts\build-release.ps1` ngay trên máy tính cá nhân. Thư mục `dist/` sẽ sinh ra 4 file nén và `checksums.txt`. Kéo thả các file này lên mục Releases trên web GitHub.
- *Cách 2 (Tự động):* Gắn tag phiên bản (ví dụ `git tag v1.0.0 && git push origin v1.0.0`), workflow `.github/workflows/release.yml` sẽ tự động biên dịch và tạo release.

---

## 7. Ràng Buộc Kỷ Luật Bắt Buộc Dành Cho Mọi AI Agent

1. **Tuyệt đối không tự ý chạy lệnh Git phá hủy hoặc tự commit:**
   - KHÔNG chạy `git commit`, `git push`, `git reset --hard`, `git clean -fd` nếu người dùng chưa ra lệnh rõ ràng bằng văn bản.
   - Để toàn bộ file sửa đổi ở trạng thái sạch trên đĩa để người dùng tự tay review và commit.
2. **Không tự biên context / Không bịa đặt giả định:**
   - Mọi thông tin tài khoản, tên tác giả, URL repository phải tuân thủ đúng định danh đã xác nhận: `kychitoge/vbook-dav`.
3. **Tuân thủ nguyên tắc SRP và cấu trúc song ngữ cho tài liệu:**
   - Mỗi file tài liệu trong `docs/` chỉ đảm nhận một chức năng duy nhất.
   - Luôn duy trì định dạng chuẩn: **Nửa trên Tiếng Việt 🇻🇳, nửa dưới Tiếng Anh 🇬🇧**.
4. **Tuân thủ chuẩn văn phong Microsoft Writing Style Guide:**
   - Sử dụng **sentence case** cho tiêu đề và nhãn giao diện.
   - Thông báo lỗi tuân thủ mô hình **Tam Giác Lỗi** (Sự việc • Nguyên nhân • Giải pháp).
   - Không đưa emoji vào terminal logs hay startup banners.
5. **Thực thi kiểm thử trước khi báo cáo hoàn thành:**
   - Mọi thay đổi mã nguồn phải được kiểm tra thực tế bằng lệnh `go test -count=1 -p 1 ./...` và `go vet ./...` đạt exit code 0.
