@echo off
title Quinos Cloud Update Manager — Local Testing
color 0b

echo ================================================================
echo   QUINOS CLOUD UPDATE MANAGER (VPS SERVER LOCAL RUNNER)
echo ================================================================
echo.

if not exist "data\packages" mkdir data\packages

if not exist "quinos-cloud-server.exe" (
    echo [BUILD] Mengompilasi quinos-cloud-server.exe...
    go build -o quinos-cloud-server.exe ./cmd/server
    if errorlevel 1 (
        echo [ERROR] Gagal kompilasi Go. Pastikan Go terpasang.
        pause
        exit /b 1
    )
)

echo [START] Menjalankan Quinos Cloud Server pada port 8080...
echo [INFO] Dashboard: http://localhost:8080/
echo.
quinos-cloud-server.exe -port :8080 -db data\quinos_cloud.db -storage data\packages -html web\index.html
pause
