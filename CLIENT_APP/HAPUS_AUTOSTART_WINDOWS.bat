@echo off
title Hapus Auto-Start Quinos Agent
color 0c

echo ================================================================
echo       MENGHAPUS AUTO-START QUINOS AGENT DARI STARTUP
echo ================================================================
echo.

set STARTUP_FOLDER=%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup
set VBS_LAUNCHER=%STARTUP_FOLDER%\QuinosAgentAutostart.vbs

if exist "%VBS_LAUNCHER%" (
    del "%VBS_LAUNCHER%"
    echo [SUKSES] File autostart di startup telah dihapus.
) else (
    echo [INFO] Shortcut autostart memang tidak ada di Startup.
)

echo.
pause
