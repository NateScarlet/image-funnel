@echo off
cd /d "%~dp0"
echo 正在启动 ImageFunnel...
echo Starting ImageFunnel...

:: Check if PowerShell is available (it should be)
where powershell >nul 2>nul
if %errorlevel% neq 0 (
    echo Error: PowerShell is required but not found.
    pause
    exit /b 1
)

:: Run the launcher script
powershell -NoProfile -ExecutionPolicy Bypass -File ".\launcher.ps1"

if %errorlevel% neq 0 (
    echo.
    echo An error occurred.
    pause
)
