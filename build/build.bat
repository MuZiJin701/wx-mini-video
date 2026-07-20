@echo off
setlocal enabledelayedexpansion

set ROOT_DIR=%~dp0..
for %%I in ("%ROOT_DIR%") do set ROOT_DIR=%%~fI
set DIST_DIR=%ROOT_DIR%\dist\wx-mini-video-windows-amd64
set ZIP_PATH=%ROOT_DIR%\dist\wx-mini-video-windows-amd64.zip

if "%1" equ "" goto usage
goto %1

:windows
call :prepare
echo Building wx-mini-video Windows amd64...
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags="-s -w -X main.Mode=release" -o "%DIST_DIR%\wx-mini-video.exe" "%ROOT_DIR%"
if errorlevel 1 exit /b 1
copy /Y "%ROOT_DIR%\internal\config\config.template.yaml" "%DIST_DIR%\wx-mini-video.yaml" >nul
copy /Y "%ROOT_DIR%\README.md" "%DIST_DIR%\README.md" >nul
powershell -NoProfile -ExecutionPolicy Bypass -Command "$ErrorActionPreference='Stop'; Compress-Archive -Path '%DIST_DIR%\*' -DestinationPath '%ZIP_PATH%' -Force"
if errorlevel 1 exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT_DIR%\build\verify-package.ps1" -ZipPath "%ZIP_PATH%"
if errorlevel 1 exit /b 1
echo Done: %ZIP_PATH%
exit /b 0

:prepare
if exist "%DIST_DIR%" rmdir /S /Q "%DIST_DIR%"
mkdir "%DIST_DIR%"
if exist "%ZIP_PATH%" del /Q "%ZIP_PATH%"
exit /b 0

:usage
echo Usage: build.bat [target]
echo   windows      - portable Windows amd64 zip without ffmpeg
exit /b 1
