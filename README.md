# Quinos Cloud Update Manager (VPS Edition)

Platform terpusat berbasis Cloud VPS untuk mendistribusikan pembaruan aplikasi Quinos POS (`.jar`) secara aman, terkontrol, dan otomatis ke seluruh cabang restoran.

---

## 🌟 Fitur Utama

- **Multi-Outlet & Multi-Terminal Hierarchy:** Manajemen terstruktur per cabang (`OUT-JKT-01`, `OUT-SBY-01`, dll.) dan per kasir (`POS-01`, `POS-02`).
- **Canary & Ring Rollout:** Rilis bertahap:
  - `Canary`: Uji coba ke 1-2 toko percontohan.
  - `Pilot`: Rilis ke grup cabang terbatas.
  - `Stable`: Rilis massal ke seluruh cabang.
- **Resumable Download (Chunking):** Mendukung *HTTP 206 Partial Content* via HTTP Range Request sehingga unduhan file `.jar` besar (40-100MB+) dapat dilanjutkan jika internet cabang terputus.
- **Verifikasi Integritas SHA-256:** Otomatis menghitung dan memvalidasi checksum sebelum penggantian file.
- **Proteksi Transaksi Kasir & Auto-Rollback:** Memastikan kasir tidak dimatikan paksa saat sedang melayani transaksi, serta rollback otomatis jika aplikasi baru mengalami crash saat startup.
- **Dark Modern Web Dashboard:** Konsol monitoring real-time berbasis web untuk melihat status koneksi, proses Java kasir, riwayat task, dan audit log.

---

## 📁 Struktur Direktori

```text
CLOUD UPDATE MANAGER/
├── cmd/
│   └── server/
│       └── main.go              # Entry point server cloud
├── internal/
│   ├── models/                  # Struct model (Outlet, Terminal, Package, Deployment, Task, Log)
│   ├── db/                      # Database engine SQLite (CGO-Free) & migrasi otomatis
│   └── server/                  # REST API Router, Agent handlers, Admin handlers
├── web/
│   └── index.html               # Web Admin Dashboard (React 18 + Tailwind single file)
├── nginx/
│   └── default.conf             # Konfigurasi reverse proxy Nginx untuk VPS
├── Dockerfile                   # Multi-stage lightweight Linux binary container
├── docker-compose.yml           # Orkestrasi Nginx + Cloud Server
├── setup-vps.sh                 # Skrip instalasi otomatis di Ubuntu/Debian VPS
├── start-cloud-server.bat       # Launcher lokal di Windows untuk pengujian
├── go.mod                       # Modul Go & dependensi
└── README.md
```

---

## 🚀 Cara Menjalankan

### Opsi A: Pengujian Lokal di Komputer Windows
1. Jalankan skrip:
   ```cmd
   start-cloud-server.bat
   ```
2. Buka browser:
   - Dashboard Admin: `http://localhost:8080/`
   - Overview API: `http://localhost:8080/api/v1/admin/overview`

### Opsi B: Deployment di Cloud VPS (Ubuntu / Debian)
1. Salin folder `CLOUD UPDATE MANAGER` ke VPS (misal di `/opt/quinos-cloud/`):
   ```bash
   scp -r "CLOUD UPDATE MANAGER" user@vps_ip:/opt/quinos-cloud
   ```
2. Masuk ke VPS via SSH dan jalankan setup:
   ```bash
   cd /opt/quinos-cloud
   chmod +x setup-vps.sh
   ./setup-vps.sh
   ```
3. Akses dashboard via IP publik VPS Anda di port 80/8080 atau melalui domain Anda.
