@echo off
title Quinos Cloud Update Manager + Cloudflare Tunnel
color 0a

echo ==============================================================================
echo        QUINOS CLOUD UPDATE MANAGER - CLOUDFLARE SECURE TUNNEL
echo ==============================================================================
echo.
echo [1/3] Menyiapkan direktori penyimpanan lokal...
if not exist "data\packages" mkdir data\packages

echo [2/3] Menjalankan Quinos Cloud Server (Port :8080)...
start "Quinos Cloud Server Engine" /min quinos-cloud-server.exe -port :8080 -db data\quinos_cloud.db -storage data\packages -html web\index.html

timeout /t 2 /nobreak >nul

echo [3/3] Membuka Cloudflare Tunnel ke Internet Publik...
echo ==============================================================================
echo  TUNGGU BEBERAPA DETIK SAMPAI MUNCUL LINK:
echo  https://xxxx.trycloudflare.com
echo.
echo  Link tersebut adalah URL HTTPS Publik Anda yang bisa diakses dari SELURUH DUNIA
echo  tanpa perlu sewa VPS dan tanpa kartu kredit!
echo ==============================================================================
echo.

cloudflared tunnel --url http://localhost:8080

pause
