# [ADR-001] Zero-Disk Streaming and on-the-fly encryption
- **Ngày:** 2026-09-27
- **Trạng thái:** Bị thay thế bởi ADR-005
- **Bối cảnh:** Thiết kế ban đầu: stream 8KB → mã hóa AES-256-GCM trên RAM → buffer 5MB `sync.Pool` → đẩy song song lên Cloudflare R2 qua S3 Multipart. Không ghi đĩa.
- **Quyết định:** Xây dựng Gateway như Streaming Proxy không lưu đĩa.
- **Hệ quả:** Phức tạp, phụ thuộc mạng quốc tế, dễ timeout. Đã thay thế bằng mô hình Local-First.

---

# [ADR-002] Dị biệt WebDAV trên vBook Client
- **Ngày:** 2026-09-27
- **Trạng thái:** Đã duyệt
- **Bối cảnh:** vBook có 4 hành vi lệch chuẩn RFC 4918 gây lỗi nếu dùng thư viện WebDAV mặc định.
- **Quyết định:**
  1. `MKCOL` thư mục đã có → trả `201 Created` (idempotent)
  2. `PUT` vào đường dẫn sâu → tự động `MkdirAll` thư mục cha
  3. `OPTIONS` → bypass auth, trả `200` kèm `DAV: 1, 2`
  4. Log đầy đủ method, path, client IP, status code cho mọi request
- **Hệ quả:** Tương thích 100% với vBook, đã kiểm chứng thực tế.

---

# [ADR-003] Hạ tầng $0 và kiểm thử độ bền kết nối
- **Ngày:** 2026-09-27
- **Trạng thái:** Đã duyệt
- **Bối cảnh:** Container miễn phí thường có Cold Start và giới hạn timeout HTTP.
- **Quyết định:**
  1. Ưu tiên nền tảng không yêu cầu thẻ tín dụng
  2. Cold Start < 20 giây, Connection Lifetime > 5 phút
  3. Kiểm thử bằng `curl --limit-rate` trước khi đưa vào sử dụng
- **Hệ quả:** Tiêu chuẩn đo kiểm rõ ràng cho việc chọn hosting.

---

# [ADR-004] Multi-Tenant 8-10 user, Quota 1GB, R2 10GB Free Tier
- **Ngày:** 2026-09-27
- **Trạng thái:** Bị thay thế bởi ADR-005
- **Bối cảnh:** Thiết kế ban đầu hướng tới 8-10 user với Database (Turso/D1), phân quyền, Hard Quota.
- **Quyết định:** Multi-Tenant + CSDL + Quota.
- **Hệ quả:** Over-engineering cho nhu cầu self-host 1 user. Đã thay thế.

---

# [ADR-005] MVP v1.0: Single-user, Local-First, Async R2 Replica, Discord notification
- **Ngày:** 2026-10-29
- **Trạng thái:** Đã duyệt
- **Bối cảnh:**
  1. MVP v1.0 phục vụ 1 user duy nhất, self-host trên máy cá nhân (Docker hoặc bare-metal).
  2. Thực nghiệm cho thấy ghi đĩa nội bộ qua Wi-Fi 5GHz đạt 13.7 MB/s (87 MB trong 6.4 giây). Lưu local trước mang lại trải nghiệm tức thì.
  3. Cloudflare R2 dùng làm bản sao dự phòng, không phải storage chính.
- **Quyết định:**
  1. **Local-First Write-Behind:** `PUT` → ghi đĩa local → trả `201` ngay → worker đẩy ngầm lên R2.
  2. **Không Database:** Xác thực 1 user qua `.env` (`DAV_USER`, `DAV_PASS`).
  3. **Không mã hóa trên R2:** Đẩy nguyên file `.tar.zst` để tiện truy cập thủ công.
  4. **Atomic Rename:** Ghi `.tmp` → `Sync()` → `os.Rename`. Không bao giờ đẩy file ghi dở lên R2.
  5. **Dọn dẹp R2 theo vBook:** vBook tự quản lý 5 bản backup. Khi vBook gửi `DELETE` → server xóa local và xóa trên R2.
  6. **Startup:** Dọn `.tmp` còn sót. Scan local so với R2, đẩy bù file thiếu.
  7. **Discord Webhook:** Thông báo upload/delete/lỗi R2. Cấu hình qua `DISCORD_WEBHOOK_URL`. Tùy chọn, không bắt buộc.
  8. **Graceful fallback:** Không cấu hình R2 → chạy local-only. Không cấu hình Discord → không gửi thông báo. Không crash.
- **Đã cắt bỏ (dao cạo):**
  - Magic Bytes check (Atomic Rename đủ đảm bảo toàn vẹn)
  - Startup Auto-Hydration (đĩa rỗng là sự kiện hiếm khi self-host với Docker volume)
  - Nâng buffer 8KB → 64KB (nút thắt ở mạng, không phải buffer)
  - Telegram Bot (phức tạp hơn Discord Webhook, MVP chỉ cần 1 kênh)
- **Hệ quả:**
  - *Lợi ích:* Tốc độ vBook tức thì, code tinh gọn, không cõng DB, fallback an toàn.
  - *Đánh đổi:* Chưa hỗ trợ nhiều user. Khi đĩa rỗng (máy mới), cần thao tác thủ công để kéo dữ liệu từ R2.

---

# [ADR-006] Chuẩn bị Open-Source & Tự Triển Khai (Self-Host Ready)
- **Ngày:** 2026-10-30
- **Trạng thái:** Đã duyệt
- **Bối cảnh:** Chuẩn bị mở mã nguồn dự án `vbook-dav` cho cộng đồng lập trình viên tự host với độ tin cậy và tính thẩm mỹ cao nhất.
- **Quyết định:**
  1. **Tài liệu chuẩn SRP (Single Responsibility Principle):** Tách bạch rõ 5 tài liệu độc lập (`architecture.md`, `configuration.md`, `self-hosting.md`, `vbook-protocol.md`, `troubleshooting.md`), song ngữ nửa trên Tiếng Việt, nửa dưới Tiếng Anh.
  2. **Đóng gói Docker chuẩn Production:** Multi-stage build (Alpine 3.21), non-root user (UID/GID 1000:1000), security hardening (`no-new-privileges: true`, `cap_drop: ALL`), tích hợp probe kiểm tra sức khỏe `/healthz`.
  3. **Giao diện Terminal Tối Giản (Minimalist Zinc):** Loại bỏ emoji, bảng box ANSI chuẩn xác, tự động nhận diện IP LAN và hướng dẫn cấu hình vBook trực quan.
  4. **Hạ tầng Release Đa Nền Tảng Tinh Giản:** Tránh phức tạp hóa CI/CD dễ lỗi vặt; cung cấp script cục bộ `scripts/build-release.ps1` cross-compile trong 30 giây cho 4 OS phổ biến nhất (Linux amd64/arm64, Windows amd64, macOS arm64) kèm SHA256 checksums, kết hợp workflow GitHub Actions 1 job tối giản.
  5. **Định danh Repository:** `https://github.com/kychitoge/vbook-dav`.
- **Hệ quả:**
  - *Lợi ích:* Dự án chuyên nghiệp, tài liệu rõ ràng, người dùng tải về là chạy ngay, an toàn bảo mật cao.
  - *Đánh đổi:* Cần duy trì tính đồng bộ giữa các file tài liệu song ngữ khi có thay đổi logic lớn.
