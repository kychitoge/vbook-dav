# Hướng Dẫn Xử Lý Sự Cố & Chẩn Đoán / Troubleshooting & Diagnostic Guide

[Tiếng Việt](#-tiếng-việt) • [English](#-english)

---

<a id="-tiếng-việt"></a>
## 🇻🇳 Tiếng Việt

Tài liệu này cung cấp quy trình chẩn đoán và khắc phục sự cố theo chuẩn **Mô hình Tam Giác Lỗi** (Sự việc • Nguyên nhân • Giải pháp) của Microsoft Style Guide.

---

### 1. Ứng dụng vBook Báo Lỗi "Không Thể Kết Nối" hoặc "Timeout" Khi Kiểm Tra Kết Nối

- **Sự việc**: Khi chọn "Kiểm tra kết nối" trong ứng dụng vBook, ứng dụng hiển thị lỗi timeout hoặc không tìm thấy máy chủ.
- **Nguyên nhân**:
  1. Điện thoại và máy chủ không kết nối chung một mạng Wi-Fi (LAN).
  2. Địa chỉ IP máy chủ bị thay đổi do DHCP của router.
  3. Tường lửa (Firewall) trên máy chủ đang chặn cổng TCP `8080`.
  4. URL trong ứng dụng thiếu dấu gạch chéo `/` ở cuối.
- **Giải pháp**:
  1. Xác nhận điện thoại đang kết nối vào cùng mạng Wi-Fi với máy chủ (tắt mạng di động 4G/5G khi thử nghiệm).
  2. Kiểm tra lại địa chỉ IP hiển thị trên banner terminal của `vbook-dav` khi khởi động (ví dụ: `http://192.168.1.50:8080/`).
  3. Mở cổng trên tường lửa:
     - **Linux (ufw)**: `sudo ufw allow 8080/tcp`
     - **Windows Defender Firewall**: Mở PowerShell với quyền Administrator và chạy:
       ```powershell
       New-NetFirewallRule -DisplayName "vbook-dav 8080" -Direction Inbound -LocalPort 8080 -Protocol TCP -Action Allow
       ```
  4. Đảm bảo ô **URL** trong vBook có định dạng đầy đủ kèm dấu gạch chéo cuối: `http://<IP-MAY-CHU>:8080/`.

---

### 2. Lỗi HTTP 401 Unauthorized (Xác Thực Thất Bại)

- **Sự việc**: Ứng dụng vBook báo lỗi xác thực thất bại hoặc máy chủ ghi nhận log `[auth ] ... 401`.
- **Nguyên nhân**: Tên tài khoản (`Username`) hoặc mật khẩu (`Password`) nhập trong ứng dụng không trùng khớp với giá trị `DAV_USER` hoặc `DAV_PASS` trên máy chủ.
- **Giải pháp**:
  1. Mở tệp `.env` trên máy chủ để xem lại thông tin đăng nhập:
     ```bash
     cat .env
     ```
  2. Mở ứng dụng vBook > **Cài đặt** > **Đồng bộ & sao lưu** > **WebDAV**, nhập lại chính xác tài khoản và mật khẩu. Chú ý phân biệt chữ hoa/chữ thường và không để thừa khoảng trắng.

---

### 3. Lỗi Docker Linux: "Permission Denied" Khi Ghi Dữ Liệu Vào `./data`

- **Sự việc**: Container `vbook-dav` khởi động thất bại hoặc ứng dụng báo lỗi HTTP 500 khi upload tệp sao lưu. Trong log Docker xuất hiện lỗi: `failed to create storage directory ... permission denied`.
- **Nguyên nhân**: Container chạy với tài khoản bảo mật không đặc quyền (UID 1000:GID 1000), nhưng thư mục mount `./data` trên máy chủ host Linux lại thuộc quyền sở hữu của tài khoản `root`.
- **Giải pháp**:
  Phân quyền lại thư mục `./data` trên máy chủ host về UID 1000:
  ```bash
  sudo chown -R 1000:1000 ./data
  sudo chmod -R 755 ./data
  ```
  Sau đó khởi động lại container:
  ```bash
  docker compose restart
  ```

---

### 4. Lỗi HTTP 403 Forbidden Khi Upload Bản Sao Lưu

- **Sự việc**: Khi vBook tiến hành sao lưu, máy chủ phản hồi mã lỗi `403 Forbidden` và từ chối ghi tệp.
- **Nguyên nhân**: Máy chủ đang được bật chế độ chỉ đọc thông qua biến môi trường `READ_ONLY=true`.
- **Giải pháp**:
  Mở tệp `.env` (hoặc kiểm tra biến môi trường của container), đổi giá trị thành `READ_ONLY=false`, sau đó khởi động lại máy chủ.

---

### 5. Cảnh Báo "Cloud Replication Queue Is Full" hoặc Lỗi Đồng Bộ R2

- **Sự việc**: Trong log xuất hiện cảnh báo `[warn ] Cloud replication queue is full` hoặc Discord báo lỗi đồng bộ R2 thất bại.
- **Nguyên nhân**:
  1. Thông tin xác thực Cloudflare R2 (`R2_ACCESS_KEY`, `R2_SECRET_KEY`) không chính xác hoặc đã bị thu hồi.
  2. Tên thùng chứa `R2_BUCKET` không tồn tại trên tài khoản Cloudflare.
  3. Đường truyền internet từ máy chủ đến Cloudflare bị mất kết nối hoặc chập chờn.
- **Giải pháp**:
  1. Kiểm tra lại thông tin R2 trong Cloudflare Dashboard: **R2** > **Manage R2 API Tokens**.
  2. Kiểm tra lại URL `R2_ENDPOINT` (đảm bảo đúng định dạng `https://<ACCOUNT_ID>.r2.cloudflarestorage.com`, không có dấu `/` ở cuối).
  3. Xác nhận bucket đã được tạo sẵn trên Cloudflare R2.

---

### 6. Xung Đột Cổng "Address Already in Use"

- **Sự việc**: Máy chủ báo lỗi `bind: address already in use` và dừng hoạt động ngay khi vừa khởi chạy.
- **Nguyên nhân**: Cổng mạng được cấu hình (mặc định là `8080`) đang bị chiếm dụng bởi một tiến trình khác trên máy tính.
- **Giải pháp**:
  - **Cách 1: Đổi cổng của vbook-dav**:
    Trong `.env`, đổi sang cổng khả dụng khác (ví dụ `PORT=:9000` hoặc chạy `vbook-dav -port :9000`).
  - **Cách 2: Giải phóng cổng bị chiếm**:
    - *Windows*: Tìm tiến trình chiếm cổng và dừng lại:
      ```powershell
      Get-NetTCPConnection -LocalPort 8080 | Select-Object OwningProcess
      Stop-Process -Id <PID> -Force
      ```
    - *Linux*:
      ```bash
      sudo fuser -k 8080/tcp
      ```

---

### 7. Kiểm Tra Sức Khỏe Dịch Vụ Với Endpoint `/healthz`

Máy chủ cung cấp một endpoint kiểm tra sức khỏe siêu nhẹ tại `/healthz`:
- Không yêu cầu Basic Auth.
- Không ghi log rác gây nghẽn console.
- Phản hồi `200 OK` kèm văn bản `ok` nếu tiến trình máy chủ đang hoạt động bình thường.

#### Cách kiểm tra nhanh bằng dòng lệnh:
```bash
# Sử dụng curl
curl -i http://localhost:8080/healthz

# Sử dụng wget
wget -q -O - http://localhost:8080/healthz
```

---

<a id="-english"></a>
## 🇬🇧 English

This document provides troubleshooting procedures following the Microsoft Writing Style Guide **Error Triad** model: **What happened** • **Why** • **Action to take**.

---

### 1. vBook Reports "Connection Failed" or "Timeout" During Connection Test

- **What happened**: When selecting "Test connection" in the vBook settings, the app reports a connection timeout or cannot reach the server.
- **Why**:
  1. The mobile device and server are on different local networks (Wi-Fi mismatch).
  2. The server's LAN IP address changed due to DHCP lease renewal.
  3. The host firewall is blocking inbound TCP traffic on port `8080`.
  4. The URL field in the vBook app is missing the required trailing slash `/`.
- **Action to take**:
  1. Ensure both devices are connected to the same Wi-Fi network (disable cellular data temporarily to verify).
  2. Confirm the active IP address printed in the `vbook-dav` startup console banner (for example, `http://192.168.1.50:8080/`).
  3. Allow TCP port `8080` through the firewall:
     - **Linux (ufw)**: `sudo ufw allow 8080/tcp`
     - **Windows Defender Firewall**: Open PowerShell as Administrator and run:
       ```powershell
       New-NetFirewallRule -DisplayName "vbook-dav 8080" -Direction Inbound -LocalPort 8080 -Protocol TCP -Action Allow
       ```
  4. Verify the **URL** in vBook ends with a trailing slash: `http://<SERVER-IP>:8080/`.

---

### 2. HTTP 401 Unauthorized Error

- **What happened**: vBook displays an authentication failure error, and the server logs `[auth ] ... 401`.
- **Why**: The username or password entered in the app does not match `DAV_USER` or `DAV_PASS` in the server environment configuration.
- **Action to take**:
  1. Inspect the credentials configured in `.env`:
     ```bash
     cat .env
     ```
  2. In the vBook app, navigate to **Settings** > **Backup and sync** > **WebDAV**, and re-enter the credentials. Credentials are case-sensitive; check for trailing whitespace.

---

### 3. Docker Linux Error: "Permission Denied" Writing to `./data`

- **What happened**: The container fails to start, or backup uploads fail with HTTP 500. Container logs report `failed to create storage directory ... permission denied`.
- **Why**: The container process runs under an unprivileged user (UID 1000:GID 1000), but the mounted `./data` directory on the Linux host is owned by `root`.
- **Action to take**:
  Update ownership of the host directory to UID 1000:
  ```bash
  sudo chown -R 1000:1000 ./data
  sudo chmod -R 755 ./data
  ```
  Restart the container:
  ```bash
  docker compose restart
  ```

---

### 4. HTTP 403 Forbidden During Backup Upload

- **What happened**: The client fails to upload a backup archive, and the server responds with `403 Forbidden`.
- **Why**: The server has read-only mode enabled (`READ_ONLY=true`).
- **Action to take**:
  In your `.env` file or container environment, set `READ_ONLY=false` and restart the server.

---

### 5. "Cloud Replication Queue Is Full" or R2 Sync Failures

- **What happened**: The console outputs `[warn ] Cloud replication queue is full`, or Discord alerts indicate sync failure.
- **Why**:
  1. Cloudflare R2 credentials (`R2_ACCESS_KEY`, `R2_SECRET_KEY`) are invalid or revoked.
  2. The target `R2_BUCKET` does not exist in the Cloudflare dashboard.
  3. Outbound internet connectivity from the host is interrupted.
- **Action to take**:
  1. Verify credentials in Cloudflare: **R2** > **Manage R2 API Tokens**.
  2. Verify `R2_ENDPOINT` format (must be `https://<ACCOUNT_ID>.r2.cloudflarestorage.com` without a trailing slash).
  3. Ensure the target bucket is created and active in Cloudflare R2.

---

### 6. Port Conflict: "Address Already in Use"

- **What happened**: The server exits immediately on startup with `bind: address already in use`.
- **Why**: Another process on the system is already bound to TCP port `8080`.
- **Action to take**:
  - **Option 1: Change port**: Set `PORT=:9000` in `.env` or run `vbook-dav -port :9000`.
  - **Option 2: Terminate conflicting process**:
    - *Windows*:
      ```powershell
      Get-NetTCPConnection -LocalPort 8080 | Select-Object OwningProcess
      Stop-Process -Id <PID> -Force
      ```
    - *Linux*:
      ```bash
      sudo fuser -k 8080/tcp
      ```

---

### 7. Health Check Verification Using `/healthz`

The server provides a lightweight monitoring endpoint at `/healthz`:
- Bypasses Basic Authentication.
- Does not clutter console logs.
- Returns `200 OK` with text body `ok` when the server is healthy.

#### Quick command-line test:
```bash
# Using curl
curl -i http://localhost:8080/healthz

# Using wget
wget -q -O - http://localhost:8080/healthz
```
