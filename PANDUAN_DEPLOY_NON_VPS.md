# Panduan Deploy Cloud Update Manager Tanpa VPS (PaaS & Cloud Container)

Dokumen ini menjelaskan opsi deployment jika Anda **tidak ingin mengelola VPS Linux manual** (tanpa setup Ubuntu, SSH, UFW, Nginx, dan certbot sendiri).

---

## 1. Apakah Bisa Deploy di Vercel dengan Docker?

> ❌ **Jawabannya: TIDAK BISA.**  
> Vercel **tidak mendukung** custom Dockerfile / container deployment.

### Mengapa Vercel Tidak Cocok untuk Quinos Update Manager?
1. **Tidak Ada Dukungan Docker:** Vercel dirancang murni untuk framework frontend (Next.js, React, Vite) dan Serverless Functions (AWS Lambda).
2. **Limit Ukuran Upload (Max 4.5 MB):** Vercel Serverless Function memiliki batas payload request maksimal **4.5 MB**. File `quinos.jar` berukuran 40–100 MB sehingga akan langsung ditolak (`413 Payload Too Large`).
3. **Penyimpanan Bersifat Sementara (Ephemeral):** Vercel tidak memiliki persistent disk. File database SQLite (`.db`) dan file `.jar` yang Anda upload akan hilang setiap kali serverless function melakukan cold restart.
4. **Batas Waktu Eksekusi (Timeout):** Fungsi serverless memiliki batas waktu eksekusi singkat (10 detik pada akun gratis). Jika kasir di cabang mendownload JAR 60 MB dengan internet lambat, koneksi akan diputus otomatis oleh Vercel.

---

## 2. Solusi Terbaik Tanpa VPS: PaaS Berbasis Docker (Zero-Maintenance)

Platform **PaaS (Platform as a Service)** berikut **100% mendukung Dockerfile**, menyediakan persistent storage untuk file JAR & SQLite, gratis atau sangat murah ($0 - $5/bulan), dan otomatis menyediakan domain HTTPS/SSL tanpa Anda harus mengelola server Linux.

| Platform | Dukungan Docker | Persistent Disk (JAR + SQLite) | URL HTTPS Otomatis | Kemudahan Setup |
|---|---|---|---|---|
| **Render.com** (Rekomendasi #1) | ✅ Ya (Otomatis dari Dockerfile) | ✅ Ya (Persistent Disk) | ✅ Ya (Gratis) | ⭐⭐⭐⭐⭐ (Sangat Mudah) |
| **Railway.app** (Rekomendasi #2) | ✅ Ya (1-Click Docker) | ✅ Ya (Volume Mount) | ✅ Ya (Gratis) | ⭐⭐⭐⭐⭐ (Sangat Mudah) |
| **Fly.io** | ✅ Ya (Via CLI `fly deploy`) | ✅ Ya (Fly Volumes) | ✅ Ya (Gratis) | ⭐⭐⭐⭐ (Pakai Terminal) |

---

## 3. Panduan Deploy ke Render.com (Langkah demi Langkah)

Render.com adalah opsi paling praktis karena memiliki UI web yang bersih dan otomatis mendeteksi `Dockerfile` yang sudah kita buat.

### Langkah 1: Push Folder ke GitHub
1. Buat repositori baru di GitHub (misal: `quinos-cloud-update-manager`).
2. Masukkan isi folder `CLOUD UPDATE MANAGER` ke repositori tersebut:
   ```bash
   git init
   git add .
   git commit -m "Initial Cloud Update Manager"
   git remote add origin https://github.com/username/quinos-cloud-update-manager.git
   git push -u origin main
   ```

### Langkah 2: Buat Web Service di Render
1. Buka [https://render.com](https://render.com) dan login dengan akun GitHub.
2. Klik tombol **New +** -> Pilih **Web Service**.
3. Pilih repositori GitHub `quinos-cloud-update-manager` yang baru saja Anda buat.
4. Konfigurasikan pengaturan:
   - **Name:** `quinos-update-hub` (atau nama lain)
   - **Region:** Singapore (`Southeast Asia - Singapore`) agar latensi ke kasir di Indonesia sangat cepat.
   - **Environment:** `Docker` (Render otomatis mendeteksi `Dockerfile` yang ada di root folder).
   - **Plan:** Free atau Starter ($7/bln).

### Langkah 3: Pasang Persistent Disk (Wajib untuk Simpan JAR & DB)
1. Pada menu navigasi service di Render, klik tab **Disks**.
2. Klik **Add Disk**:
   - **Name:** `quinos-data`
   - **Mount Path:** `/app/data`
   - **Size:** 10 GB (cukup untuk puluhan versi JAR dan database).
3. Klik **Save**.

### Langkah 4: Simpan & Selesai!
- Render akan otomatis melakukan build Docker image, menyalakan server, dan memberikan URL HTTPS gratis:
  `https://quinos-update-hub.onrender.com`
- Buka URL tersebut di browser untuk mengakses **Quinos Cloud Web Dashboard**.
- Di file `agent.json` pada PC Kasir, Anda cukup mengganti server URL menjadi URL Render tersebut:
  ```json
  {
    "server_url": "https://quinos-update-hub.onrender.com",
    "outlet_code": "OUT-JKT-01",
    "api_key": "key_senopati_secret123"
  }
  ```

---

## 4. Panduan Deploy ke Railway.app (Alternatif Cepat)

1. Buka [https://railway.app](https://railway.app) dan login dengan GitHub.
2. Klik **New Project** -> **Deploy from GitHub repo**.
3. Pilih repo `quinos-cloud-update-manager`.
4. Tambahkan Volume:
   - Klik kanan pada canvas project -> Pilih **Volume**.
   - Hubungkan volume ke service dengan Mount Path: `/app/data`.
5. Buka tab **Settings** -> **Networking** -> Klik **Generate Domain**.
6. Railway akan memberikan domain publik HTTPS (misal: `quinos-update.up.railway.app`).

---

## 5. Ringkasan Rekomendasi

- Jika ingin **tanpa menyentuh terminal server sama sekali**: Gunakan **Render.com** (Region Singapore) dengan Persistent Disk.
- Jika ingin **biaya terendah dan performa maksimal untuk jangka panjang (skala 50+ resto)**: Gunakan **Cloud VPS (DigitalOcean / Biznet GIO)** menggunakan skrip otomatis `./setup-vps.sh` yang sudah disediakan.
