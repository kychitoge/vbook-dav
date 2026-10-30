# Hướng Dẫn Tự Triển Khai & Vận Hành / Self-Hosting & Operations Guide

[Tiếng Việt](#-tiếng-việt) • [English](#-english)

---

<a id="-tiếng-việt"></a>
## 🇻🇳 Tiếng Việt

Tài liệu này cung cấp các công thức triển khai `vbook-dav` tối ưu nhất cho máy chủ cá nhân, máy chủ ảo (VPS), Raspberry Pi hoặc thiết bị lưu trữ NAS.

---

### 1. Triển Khai Bằng Docker Compose (Khuyến Nghị Hàng Đầu)

Docker Compose là phương án nhanh chóng, an toàn và dễ bảo trì nhất. Container được cấu hình chạy dưới tài khoản không đặc quyền (non-root UID 1000) và tích hợp sẵn probe kiểm tra sức khỏe tự động.

#### Bước 1: Tải mã nguồn và chuẩn bị cấu hình
```bash
git clone https://github.com/kychitoge/vbook-dav.git
cd vbook-dav
cp .env.example .env
```

#### Bước 2: Cấu hình thông tin đăng nhập
Mở tệp `.env` bằng trình soạn thảo và tùy chỉnh tài khoản/mật khẩu:
```bash
nano .env
```
Thiết lập tối thiểu:
```env
PORT=:8080
DATA_DIR=/data
DAV_USER=vbook
DAV_PASS=MatKhauBaoMatCuaBan
```

#### Bước 3: Phân quyền thư mục dữ liệu trên Linux host
```bash
mkdir -p ./data
sudo chown -R 1000:1000 ./data
sudo chmod -R 755 ./data
```

#### Bước 4: Khởi động dịch vụ
```bash
docker compose up -d
```

#### Bước 5: Kiểm tra trạng thái và nhật ký hoạt động
```bash
docker compose ps
docker compose logs -f
```

---

### 2. Triển Khai Native Bằng Dịch Vụ Linux systemd

Dành cho thiết bị tài nguyên siêu nhẹ (Raspberry Pi Zero, Orange Pi, VPS 512 MB RAM) không muốn cài đặt Docker:

#### Bước 1: Tải file nhị phân dựng sẵn hoặc biên dịch từ mã nguồn
```bash
# Cách A: Tải bản binary dựng sẵn từ GitHub Releases (Khuyên dùng)
# Đối với Linux x86_64:
curl -fsSL -o /tmp/vbook-dav.tar.gz https://github.com/kychitoge/vbook-dav/releases/latest/download/vbook-dav_v1.0.0_linux_amd64.tar.gz
sudo tar -xzf /tmp/vbook-dav.tar.gz -C /usr/local/bin vbook-dav
sudo chmod +x /usr/local/bin/vbook-dav

# Hoặc đối với Raspberry Pi (Linux ARM64):
# curl -fsSL -o /tmp/vbook-dav.tar.gz https://github.com/kychitoge/vbook-dav/releases/latest/download/vbook-dav_v1.0.0_linux_arm64.tar.gz
# sudo tar -xzf /tmp/vbook-dav.tar.gz -C /usr/local/bin vbook-dav

# Cách B: Tự biên dịch từ mã nguồn (yêu cầu cài đặt Go 1.24+)
go build -ldflags="-s -w" -trimpath -o /usr/local/bin/vbook-dav ./cmd/dav
sudo chmod +x /usr/local/bin/vbook-dav
```

#### Bước 2: Tạo người dùng hệ thống chuyên dụng
```bash
sudo useradd -r -s /bin/false vbook
sudo mkdir -p /var/lib/vbook-dav /etc/vbook-dav
sudo chown -R vbook:vbook /var/lib/vbook-dav /etc/vbook-dav
```

#### Bước 3: Tạo tệp cấu hình `/etc/vbook-dav/vbook.env`
```bash
sudo nano /etc/vbook-dav/vbook.env
```
Nội dung tệp:
```env
PORT=:8080
DATA_DIR=/var/lib/vbook-dav
DAV_USER=vbook
DAV_PASS=MatKhauBaoMat123
```
Phân quyền bảo mật tệp cấu hình:
```bash
sudo chmod 600 /etc/vbook-dav/vbook.env
sudo chown vbook:vbook /etc/vbook-dav/vbook.env
```

#### Bước 4: Tạo tệp dịch vụ `/etc/systemd/system/vbook-dav.service`
```bash
sudo nano /etc/systemd/system/vbook-dav.service
```
Nội dung cấu hình systemd:
```ini
[Unit]
Description=vbook-dav WebDAV Gateway
After=network.target

[Service]
Type=simple
User=vbook
Group=vbook
EnvironmentFile=/etc/vbook-dav/vbook.env
ExecStart=/usr/local/bin/vbook-dav
Restart=always
RestartSec=5s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

#### Bước 5: Kích hoạt và khởi chạy
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now vbook-dav
sudo systemctl status vbook-dav
```

---

### 3. Giải Pháp Truy Cập Từ Xa An Toàn (Remote Access)

Để điện thoại có thể sao lưu và đồng bộ khi đang dùng mạng 4G/5G hoặc ở ngoài gia đình:

#### Phương án A: Tailscale Mesh VPN (Đơn giản & Bảo mật nhất — Không cần mở cổng)
1. Cài đặt Tailscale trên máy chủ và điện thoại Android từ [tailscale.com](https://tailscale.com).
2. Lấy địa chỉ IP Tailscale của máy chủ (dạng `100.x.y.z`).
3. Trong vBook, nhập URL WebDAV: `http://100.x.y.z:8080/`.
4. Sao lưu an toàn qua đường hầm mã hóa WireGuard mà không để lộ cổng ra internet công cộng.

#### Phương án B: Cloudflare Tunnel (Tên miền riêng không cần mở cổng router)
1. Cài đặt `cloudflared` trên máy chủ lưu trữ.
2. Tạo đường hầm trỏ về `http://localhost:8080`:
   ```bash
   cloudflared tunnel create vbook
   cloudflared tunnel route dns vbook dav.tenmiencuaban.com
   cloudflared tunnel run --url http://localhost:8080 vbook
   ```
3. Trong vBook, nhập URL WebDAV: `https://dav.tenmiencuaban.com/`.

#### Phương án C: Caddy Reverse Proxy (Tự động cấp chứng chỉ HTTPS)
Nếu máy chủ có IP tĩnh công khai và mở cổng 80/443:
```caddyfile
dav.tenmiencuaban.com {
    reverse_proxy localhost:8080
}
```

---

### 4. Sao Lưu Dữ Liệu & Phục Hồi Thảm Họa (Disaster Recovery)

#### Tạo bản snapshot cục bộ
Toàn bộ dữ liệu sao lưu vBook nằm trong `DATA_DIR` (`./data/vbook_backup`). Bạn có thể tạo bản chụp nhanh:
```bash
tar -czvf vbook-snapshot-$(date +%Y%m%d).tar.gz ./data
```

#### Phục hồi từ Cloudflare R2 khi hỏng ổ cứng máy chủ
Nếu máy chủ vật lý gặp sự cố nhưng bạn đã kích hoạt tính năng đồng bộ R2:
1. Đăng nhập vào Cloudflare Dashboard > **R2**.
2. Chọn bucket của bạn và tải tệp sao lưu mới nhất (ví dụ: `vbook_backup/backup.tar.zst`).
3. Khởi dựng một máy chủ `vbook-dav` mới và đặt tệp tải về vào thư mục `./data/vbook_backup/`.
4. Mở ứng dụng vBook > **Cài đặt** > **Khôi phục từ WebDAV**.

---

<a id="-english"></a>
## 🇬🇧 English

This guide provides deployment and operations recipes for running `vbook-dav` on personal home servers, Cloud VPS, Raspberry Pi, or Network Attached Storage (NAS).

---

### 1. Docker Compose Deployment (Recommended)

Docker Compose offers the quickest, most secure deployment. The container runs as an unprivileged user (UID 1000) with container hardening and an automated health probe.

#### Step 1: Clone repository and prepare configuration
```bash
git clone https://github.com/kychitoge/vbook-dav.git
cd vbook-dav
cp .env.example .env
```

#### Step 2: Configure credentials
Open `.env` in your text editor:
```bash
nano .env
```
Minimum required configuration:
```env
PORT=:8080
DATA_DIR=/data
DAV_USER=vbook
DAV_PASS=YourStrongPassword
```

#### Step 3: Configure Linux host volume permissions
```bash
mkdir -p ./data
sudo chown -R 1000:1000 ./data
sudo chmod -R 755 ./data
```

#### Step 4: Start the service
```bash
docker compose up -d
```

#### Step 5: Verify status and logs
```bash
docker compose ps
docker compose logs -f
```

---

### 2. Native Linux systemd Service Deployment

Designed for low-resource hardware (Raspberry Pi Zero, Orange Pi, VPS with 512 MB RAM) without Docker:

#### Step 1: Download pre-built binary or compile from source
```bash
# Option A: Download pre-built release binary (Recommended)
# For Linux x86_64:
curl -fsSL -o /tmp/vbook-dav.tar.gz https://github.com/kychitoge/vbook-dav/releases/latest/download/vbook-dav_v1.0.0_linux_amd64.tar.gz
sudo tar -xzf /tmp/vbook-dav.tar.gz -C /usr/local/bin vbook-dav
sudo chmod +x /usr/local/bin/vbook-dav

# Or for Raspberry Pi (Linux ARM64):
# curl -fsSL -o /tmp/vbook-dav.tar.gz https://github.com/kychitoge/vbook-dav/releases/latest/download/vbook-dav_v1.0.0_linux_arm64.tar.gz
# sudo tar -xzf /tmp/vbook-dav.tar.gz -C /usr/local/bin vbook-dav

# Option B: Build from source (Requires Go 1.24+)
go build -ldflags="-s -w" -trimpath -o /usr/local/bin/vbook-dav ./cmd/dav
sudo chmod +x /usr/local/bin/vbook-dav
```

#### Step 2: Create a dedicated system user
```bash
sudo useradd -r -s /bin/false vbook
sudo mkdir -p /var/lib/vbook-dav /etc/vbook-dav
sudo chown -R vbook:vbook /var/lib/vbook-dav /etc/vbook-dav
```

#### Step 3: Create configuration file `/etc/vbook-dav/vbook.env`
```bash
sudo nano /etc/vbook-dav/vbook.env
```
Configuration content:
```env
PORT=:8080
DATA_DIR=/var/lib/vbook-dav
DAV_USER=vbook
DAV_PASS=YourStrongPassword
```
Secure file permissions:
```bash
sudo chmod 600 /etc/vbook-dav/vbook.env
sudo chown vbook:vbook /etc/vbook-dav/vbook.env
```

#### Step 4: Create service unit `/etc/systemd/system/vbook-dav.service`
```bash
sudo nano /etc/systemd/system/vbook-dav.service
```
Service unit configuration:
```ini
[Unit]
Description=vbook-dav WebDAV Gateway
After=network.target

[Service]
Type=simple
User=vbook
Group=vbook
EnvironmentFile=/etc/vbook-dav/vbook.env
ExecStart=/usr/local/bin/vbook-dav
Restart=always
RestartSec=5s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

#### Step 5: Enable and start service
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now vbook-dav
sudo systemctl status vbook-dav
```

---

### 3. Secure Remote Access Patterns

To enable backup synchronization over mobile cellular data (4G/5G) or outside your home network:

#### Option A: Tailscale Mesh VPN (Simplest & Most Secure — Zero Port-Forwarding)
1. Install Tailscale on your server and Android phone from [tailscale.com](https://tailscale.com).
2. Note your server's Tailscale IP address (e.g., `100.x.y.z`).
3. In the vBook app, configure the WebDAV URL as `http://100.x.y.z:8080/`.
4. Backups synchronize securely via an encrypted WireGuard mesh without exposing open ports to the internet.

#### Option B: Cloudflare Tunnel (Custom Domain Without Opening Router Ports)
1. Install `cloudflared` on your server.
2. Create a tunnel pointing to `http://localhost:8080`:
   ```bash
   cloudflared tunnel create vbook
   cloudflared tunnel route dns vbook dav.yourdomain.com
   cloudflared tunnel run --url http://localhost:8080 vbook
   ```
3. In vBook, configure the WebDAV URL as `https://dav.yourdomain.com/`.

#### Option C: Caddy Reverse Proxy (Automatic HTTPS)
For hosts with a static public IP and open ports 80/443:
```caddyfile
dav.yourdomain.com {
    reverse_proxy localhost:8080
}
```

---

### 4. Backups and Disaster Recovery

#### Creating a Local Snapshot
All vBook data is stored directly in `DATA_DIR` (`./data/vbook_backup`). Create an immediate archive snapshot:
```bash
tar -czvf vbook-snapshot-$(date +%Y%m%d).tar.gz ./data
```

#### Disaster Recovery from Cloudflare R2
If the local disk suffers hardware failure and R2 replication was active:
1. Open the Cloudflare Dashboard > **R2**.
2. Select your bucket and download the newest backup archive (e.g., `vbook_backup/backup.tar.zst`).
3. Deploy a fresh `vbook-dav` instance and copy the archive into `./data/vbook_backup/`.
4. In vBook, select **Settings** > **Restore from WebDAV**.
