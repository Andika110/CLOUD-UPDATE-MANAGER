@echo off
title Quinos Cloud Update Manager + Cloudflare Tunnel
color 0a

echo ==============================================================================
echo        QUINOS CLOUD UPDATE MANAGER - CLOUDFLARE SECURE TUNNEL
echo ==============================================================================
echo.
echo [1/3] Menyiapkan server...
taskkill /F /IM quinos-cloud-server.exe 2>nul
taskkill /F /IM cloudflared.exe 2>nul
if not exist "data\packages" mkdir data\packages
if exist "tunnel.log" del /f /q "tunnel.log"

echo [2/3] Menjalankan Quinos Cloud Server (Port :8080)...
start "Quinos Cloud Server Engine" /min quinos-cloud-server.exe -port :8080 -db data\quinos_cloud.db -storage data\packages -html web\index.html

timeout /t 2 /nobreak >nul

echo [3/3] Membuka Cloudflare Tunnel ke Internet Publik...
echo ==============================================================================
echo  TUNGGU BEBERAPA DETIK SAMPAI LINK MUNCUL...
echo ==============================================================================
echo.

cloudflared tunnel --url http://localhost:8080 --logfile tunnel.log
pause
