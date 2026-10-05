# Mobile SOCKS5 + FRP Tunnel Android App

Aplikasi Android untuk mengubah smartphone menjadi **High-Performance Residential Mobile SOCKS5 Proxy** yang otomatis terhubung ke VPS via **FRP (Fast Reverse Proxy)**.

Cocok untuk:
- Proxy Farm Seluler (Telkomsel, Indosat, XL, Tri, Smartfren)
- Residential Proxy Selling System
- Multi-client routing dengan performa tinggi (Ribuan koneksi TCP & UDP bersamaan)

---

## 🚀 Fitur Unggulan
1. **Embedded FRP Client & SOCKS5 Server (Golang Core)**:
   - Tidak perlu install aplikasi frpc terpisah.
   - Tanpa root HP.
   - Performa Goroutine I/O Multiplexing anti-lag.
2. **Dukungan Penuh TCP + UDP Associate**:
   - Mendukung browsing, video streaming, Discord/VoIP, dan UDP DNS.
3. **Anti-Throttle & Anti-Kill Engine**:
   - Android Foreground Service permanen.
   - Partial CPU WakeLock.
   - High-Performance & Low-Latency WiFi Lock.
   - Exemption Doze Mode (Battery Optimization).
4. **CI/CD Build Otomatis via GitHub Actions**:
   - Cukup `git push`, file `.apk` langsung jadi dan bisa didownload di tab GitHub Actions.

---

## 🛠️ Cara Deploy & Download APK

1. Buat repository baru di akun GitHub Anda (misal: `mobile-socks5-frp`).
2. Push seluruh folder ini ke repository Anda:
   ```bash
   git init
   git add .
   git commit -m "Initial commit high-perf mobile socks5 frp"
   git branch -M main
   git remote add origin https://github.com/rickyyanuar993/mobile-socks5-frp.git
   git push -u origin main
   ```
3. Buka tab **Actions** di repository GitHub Anda.
4. Tunggu build selesai (~3-5 menit).
5. Masuk ke workflow run terbaru, lalu download file `.apk` di bagian **Artifacts**: `Mobile-Socks5-FRP-APK`.
6. Install di HP Android Anda!

---

## 📱 Cara Pakai di HP Android
1. Buka aplikasi **Mobile SOCKS5 FRP**.
2. Masukkan parameter:
   - **VPS Address**: `YOUR_VPS_IP` (IP VPS Anda)
   - **FRP Port**: `7000`
   - **FRP Token**: Token di file `frps.toml` VPS Anda (`YOUR_FRP_TOKEN`)
   - **Remote Port**: Port publik yang ingin dialokasikan di VPS (misal: `10001`)
   - **SOCKS5 Auth**: (Opsional) Username & Password
3. Klik tombol **"Disable Battery Optimization"** sekali untuk mencegah Android mematikan proses saat layar mati.
4. Klik **START PROXY TUNNEL**.

Proxy langsung siap digunakan dari laptop / komputer luar dengan alamat:
`YOUR_VPS_IP:10001`
