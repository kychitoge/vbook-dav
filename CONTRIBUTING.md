# Hướng Dẫn Đóng Góp Mã Nguồn / Contributing Guidelines

[Tiếng Việt](#-tiếng-việt) • [English](#-english)

---

<a id="-tiếng-việt"></a>
## 🇻🇳 Tiếng Việt

Cảm ơn bạn đã quan tâm và mong muốn đóng góp cho dự án mã nguồn mở `vbook-dav`.

### 1. Nguyên Tắc & Tiêu Chuẩn Kỹ Thuật

#### Nguyên tắc dao cạo Occam (Occam's razor)
Dự án kiên định với triết lý **lõi không phụ thuộc (zero-dependency core)**. Tuyệt đối không đưa thêm các thư viện bên ngoài nếu không thực sự bắt buộc. Mọi tính năng phải phục vụ trực tiếp cho người dùng self-host cá nhân với tiêu chí ổn định, đơn giản và chi phí vận hành $0.

#### Quy chuẩn mã nguồn & Kiểm thử
Trước khi gửi đóng góp hoặc tạo Pull Request, bạn bắt buộc phải kiểm tra:
1. Toàn bộ test suite chạy thành công 100%:
   ```powershell
   # Bắt buộc cờ -p 1 trên Windows để tránh lỗi cạn kiệt paging file
   go test -count=1 -p 1 ./internal/config ./internal/storage/local ./internal/webdav ./internal/r2sync ./internal/notify
   ```
2. Rà soát lỗi tĩnh và định dạng mã nguồn chuẩn:
   ```bash
   go vet ./...
   go fmt ./...
   ```
3. Đảm bảo biên dịch thành công:
   ```bash
   go build ./cmd/dav
   ```

#### Hướng dẫn văn phong kỹ thuật (Microsoft Writing Style Guide)
Toàn bộ tài liệu, log console và thông báo lỗi phải tuân thủ chuẩn Microsoft Style Guide:
- Sử dụng **sentence case** cho tiêu đề và nhãn.
- Thông báo lỗi tuân thủ chặt chẽ mô hình **Tam Giác Lỗi** (Sự việc • Nguyên nhân • Giải pháp).
- Sử dụng động từ trung tính (`select`, `enter`, `open`).
- Không sử dụng emoji trong log console và terminal banner.

---

### 2. Quy Trình Đóng Góp (Workflow)

1. Fork repository về tài khoản cá nhân của bạn trên GitHub.
2. Tạo một nhánh mới từ nhánh `main` (ví dụ: `git checkout -b fix/upload-buffer`).
3. Thực hiện sửa đổi kèm theo test case tương ứng trong package.
4. Chạy toàn bộ kiểm thử để đảm bảo không phát sinh lỗi hồi quy (regression bug).
5. Mở Pull Request kèm mô tả rõ ràng: **Vấn đề gặp phải** và **Phương án giải quyết**.

---

<a id="-english"></a>
## 🇬🇧 English

Thank you for your interest in contributing to `vbook-dav`.

### 1. Standards and Technical Guidelines

#### Occam's Razor Philosophy
We maintain a strict **zero-dependency core** philosophy. Avoid introducing external dependencies unless strictly necessary. Every feature must serve self-hosters and single-user reliability without speculative abstractions, unnecessary databases, or maintenance overhead.

#### Code Quality and Testing
Before submitting contributions or opening a Pull Request, verify that:
1. All unit and adversarial tests pass 100%:
   ```powershell
   # The -p 1 flag is required on Windows to prevent paging file exhaustion
   go test -count=1 -p 1 ./internal/config ./internal/storage/local ./internal/webdav ./internal/r2sync ./internal/notify
   ```
2. Static analysis and code formatting checks pass:
   ```bash
   go vet ./...
   go fmt ./...
   ```
3. The binary compiles cleanly:
   ```bash
   go build ./cmd/dav
   ```

#### Microsoft Writing Style Guide Standards
All documentation, CLI logs, and error messages must adhere to the Microsoft Writing Style Guide:
- Use **sentence case** for headings and UI labels.
- Structure error messages around the **Error Triad** (What happened • Why • Action to take).
- Use input-neutral verbs (`select`, `enter`, `open`).
- Avoid emojis in terminal logs and startup banners.

---

### 2. Contribution Workflow

1. Fork the repository to your personal GitHub account.
2. Create a new branch from `main` (e.g., `git checkout -b fix/upload-buffer`).
3. Implement your changes accompanied by relevant unit tests.
4. Run the entire test suite sequentially to prevent regression bugs.
5. Open a Pull Request with a clear explanation of the problem and the proposed technical resolution.
