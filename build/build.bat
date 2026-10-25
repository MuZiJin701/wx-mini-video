@echo off
setlocal enabledelayedexpansion

set ROOT_DIR=%~dp0..
for %%I in ("%ROOT_DIR%") do set ROOT_DIR=%%~fI
set DIST_DIR=%ROOT_DIR%\dist\wx-mini-video-windows-amd64
set ZIP_PATH=%ROOT_DIR%\dist\wx-mini-video-windows-amd64.zip
set FFMPEG_CACHE=%ROOT_DIR%\dist\ffmpeg-cache

set TARGET=%~1
if "%TARGET%" equ "" set TARGET=windows
if /I "%TARGET%" equ "windows" goto windows
if /I "%TARGET%" equ "usage" goto usage
if /I "%TARGET%" equ "help" goto usage
echo Unknown target: %TARGET%
goto usage

:windows
call :prepare
if errorlevel 1 exit /b 1
call :compile
if errorlevel 1 exit /b 1
if defined WXMV_FFMPEG_DIR goto bundled_local
echo Preparing bundled ffmpeg 9.0.2...
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT_DIR%\build\fetch-ffmpeg.ps1" -CacheDir "%FFMPEG_CACHE%" -OutputDir "%DIST_DIR%\ffmpeg"
if errorlevel 1 exit /b 1
goto ffmpeg_ready
:bundled_local
echo Using prebuilt ffmpeg from %WXMV_FFMPEG_DIR%...
if not exist "%WXMV_FFMPEG_DIR%\ffmpeg.exe" (
    echo Missing %WXMV_FFMPEG_DIR%\ffmpeg.exe
    exit /b 1
)
if not exist "%WXMV_FFMPEG_DIR%\LICENSE.txt" (
    echo Missing %WXMV_FFMPEG_DIR%\LICENSE.txt
    exit /b 1
)
mkdir "%DIST_DIR%\ffmpeg" 2>nul
copy /Y "%WXMV_FFMPEG_DIR%\ffmpeg.exe" "%DIST_DIR%\ffmpeg\ffmpeg.exe" >nul
if errorlevel 1 exit /b 1
copy /Y "%WXMV_FFMPEG_DIR%\LICENSE.txt" "%DIST_DIR%\ffmpeg\LICENSE.txt" >nul
if errorlevel 1 exit /b 1
:ffmpeg_ready
copy /Y "%ROOT_DIR%\THIRD-PARTY-NOTICES.md" "%DIST_DIR%\THIRD-PARTY-NOTICES.md" >nul
if errorlevel 1 exit /b 1
call :package
if errorlevel 1 exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT_DIR%\build\verify-package.ps1" -ZipPath "%ZIP_PATH%"
if errorlevel 1 exit /b 1
echo Done: %ZIP_PATH%
exit /b 0

:compile
echo Building wx-mini-video Windows amd64...
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags="-s -w -X main.Mode=release" -o "%DIST_DIR%\wx-mini-video.exe" "%ROOT_DIR%"
if errorlevel 1 exit /b 1
copy /Y "%ROOT_DIR%\internal\config\config.template.yaml" "%DIST_DIR%\wx-mini-video.yaml" >nul
if errorlevel 1 exit /b 1
copy /Y "%ROOT_DIR%\README.md" "%DIST_DIR%\README.md" >nul
if errorlevel 1 exit /b 1
exit /b 0

:package
powershell -NoProfile -ExecutionPolicy Bypass -Command "$ErrorActionPreference='Stop'; Compress-Archive -Path '%DIST_DIR%\*' -DestinationPath '%ZIP_PATH%' -Force"
if errorlevel 1 exit /b 1
exit /b 0

:prepare
if exist "%DIST_DIR%" rmdir /S /Q "%DIST_DIR%"
mkdir "%DIST_DIR%"
if errorlevel 1 exit /b 1
if exist "%ZIP_PATH%" del /Q "%ZIP_PATH%"
exit /b 0

:usage
echo Usage: build.bat [target]
echo   windows  - complete Windows amd64 zip with bundled ffmpeg 9.0.2 (default, auto-downloads if missing)
echo   usage    - print this message
exit /b 1
