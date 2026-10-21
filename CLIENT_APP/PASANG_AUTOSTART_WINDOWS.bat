@echo off
title Pasang Auto-Start Quinos Agent
color 0a

echo ================================================================
echo     MEMASANG AUTO-START QUINOS AGENT PADA STARTUP WINDOWS
echo ================================================================
echo.

set SCRIPT_DIR=%~dp0
set TARGET_BAT=%SCRIPT_DIR%JALANKAN_CLIENT.bat
set STARTUP_FOLDER=%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup
set VBS_LAUNCHER=%STARTUP_FOLDER%\QuinosAgentAutostart.vbs

echo [INFO] Direktori Agent : %SCRIPT_DIR%
echo [INFO] Folder Startup  : %STARTUP_FOLDER%
echo.

:: Membuat file VBS di folder Startup agar berjalan di background tanpa pop-up hitam
echo Set WshShell = CreateObject("WScript.Shell") > "%VBS_LAUNCHER%"
echo WshShell.CurrentDirectory = "%SCRIPT_DIR%" >> "%VBS_LAUNCHER%"
echo WshShell.Run """%TARGET_BAT%""", 0, False >> "%VBS_LAUNCHER%"

if exist "%VBS_LAUNCHER%" (
    echo ================================================================
    echo [SUKSES] Auto-Start Quinos Agent berhasil dipasang!
    echo.
    echo Agent sekarang akan OTOMATIS berjalan di latar belakang setiap kali
    echo komputer kasir dinyalakan atau di-restart.
    echo ================================================================
) else (
    echo [ERROR] Gagal memasang shortcut di Startup folder.
)

echo.
pause
