@echo off
title Quinos POS - Update Agent Client
color 0b

echo ================================================================
echo           QUINOS POS UPDATE AGENT (CLIENT KASIR)
echo ================================================================
echo.

if not exist "agent.json" (
    echo [ERROR] File konfigurasi agent.json tidak ditemukan!
    echo Silakan pastikan file agent.json berada di folder yang sama.
    echo.
    pause
    exit /b 1
)

if not exist "quinos-agent.exe" (
    echo [ERROR] Binary quinos-agent.exe tidak ditemukan!
    echo.
    pause
    exit /b 1
)

echo [START] Menjalankan Quinos Update Agent...
echo [INFO] Membaca konfigurasi dari agent.json...
echo [INFO] Menghubungkan ke Cloud Server...
echo.

quinos-agent.exe -config agent.json

echo.
echo [AGENT STOPPED] Agent telah berhenti.
pause
