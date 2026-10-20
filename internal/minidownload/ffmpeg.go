package minidownload

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	managedFFmpegMarker    = ".wx-mini-video-ffmpeg.sha256"
	ffmpegDownloadAttempts = 3
)

var managedFFmpegURL, managedFFmpegSHA256 = ffmpegSourceForPlatform(runtime.GOOS, runtime.GOARCH)

func ffmpegSourceForPlatform(goos, arch string) (string, string) {
	if goos == "windows" {
		return "https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-08-11-13-11/ffmpeg-n8.1.2-34-g9b6c8969e0-win64-gpl-8.1.zip", "05eedc113542be39af5d0f78f0b1093bafb89c98cecf25b77e8644670293107f"
	}
	if goos == "darwin" {
		switch arch {
		case "amd64":
			return "https://ffmpeg.martin-riedl.de/download/macos/amd64/1789931006_9.0.2/ffmpeg.zip", "7c6b4125b191cbf773832dc51f424cf2b6bb7da43007d1e066f95909e47cacd4"
		case "arm64":
			return "https://ffmpeg.martin-riedl.de/download/macos/arm64/1789931890_9.0.2/ffmpeg.zip", "c8ed4c4e6978a03c485edbfe4e0a5dc2380f8a30bba5150531b31b094492d924"
		}
	}
	return "", ""
}

func managedFFmpegName() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}

type FFmpegState struct {
	State   string
	Total   int64
	Current int64
	Error   error
	Path    string
}

func EnsureFFmpeg(ctx context.Context, targetDir string, onProgress func(FFmpegState)) (string, error) {
	emit := func(state FFmpegState) {
		if onProgress != nil {
			onProgress(state)
		}
	}

	emit(FFmpegState{State: "checking"})

	if targetDir == "" {
		targetDir = "."
	}
	if path := ManagedFFmpegPath(targetDir); path != "" {
		emit(FFmpegState{State: "found", Path: path})
		return path, nil
	}

	if !canAutoInstallFFmpeg(runtime.GOOS) || managedFFmpegURL == "" {
		err := fmt.Errorf("未找到 ffmpeg，请先安装 ffmpeg 或在配置中设置 download.ffmpeg")
		emit(FFmpegState{State: "error", Error: err})
		return "", err
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return "", managedFFmpegError(fmt.Errorf("创建目录失败: %w", err))
	}

	localPath := filepath.Join(targetDir, managedFFmpegName())
	zipPath := filepath.Join(targetDir, "ffmpeg_temp.zip")
	if err := downloadFFmpegArchive(ctx, zipPath, emit); err != nil {
		return "", managedFFmpegError(err)
	}
	defer os.Remove(zipPath)

	emit(FFmpegState{State: "extracting"})
	if err := extractFFmpegExe(zipPath, localPath); err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return "", managedFFmpegError(err)
	}
	if err := writeManagedFFmpegMarker(targetDir, localPath); err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return "", managedFFmpegError(fmt.Errorf("写入 ffmpeg 校验标记失败: %w", err))
	}

	emit(FFmpegState{State: "done", Path: localPath})
	return localPath, nil
}

func downloadFFmpegArchive(ctx context.Context, zipPath string, emit func(FFmpegState)) error {
	var lastErr error
	for attempt := 0; attempt < ffmpegDownloadAttempts; attempt++ {
		retryable, err := downloadFFmpegArchiveOnce(ctx, zipPath, emit)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable || attempt == ffmpegDownloadAttempts-1 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
		}
	}
	return lastErr
}

func downloadFFmpegArchiveOnce(ctx context.Context, zipPath string, emit func(FFmpegState)) (retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, managedFFmpegURL, nil)
	if err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return false, fmt.Errorf("创建 ffmpeg 下载请求失败: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return true, fmt.Errorf("下载 ffmpeg 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("下载 ffmpeg 失败: HTTP %s", resp.Status)
		emit(FFmpegState{State: "error", Error: err})
		return resp.StatusCode >= 500, err
	}

	file, err := os.Create(zipPath)
	if err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return false, fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer file.Close()

	total := resp.ContentLength
	reader := &ffmpegProgressReader{
		Reader: resp.Body,
		Total:  total,
		OnProgress: func(current int64) {
			emit(FFmpegState{State: "downloading", Total: total, Current: current})
		},
	}
	if _, err := io.Copy(file, reader); err != nil {
		_ = os.Remove(zipPath)
		emit(FFmpegState{State: "error", Error: err})
		return true, fmt.Errorf("下载 ffmpeg 失败: %w", err)
	}
	if err := file.Close(); err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return false, fmt.Errorf("写入 ffmpeg 失败: %w", err)
	}
	if err := verifyFileSHA256(zipPath, managedFFmpegSHA256); err != nil {
		_ = os.Remove(zipPath)
		emit(FFmpegState{State: "error", Error: err})
		return false, fmt.Errorf("下载的 ffmpeg 校验失败: %w", err)
	}
	return false, nil
}

func managedFFmpegError(err error) error {
	if strings.Contains(err.Error(), "download.ffmpeg") {
		return err
	}
	return fmt.Errorf("%w；可配置 download.ffmpeg 为本地 ffmpeg 可执行文件的完整路径", err)
}

func ManagedFFmpegPath(targetDir string) string {
	path := filepath.Join(targetDir, managedFFmpegName())
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	marker, err := os.ReadFile(filepath.Join(targetDir, managedFFmpegMarker))
	parts := strings.Fields(string(marker))
	if err != nil || len(parts) != 2 || parts[0] != managedFFmpegSHA256 {
		return ""
	}
	if err := verifyFileSHA256(path, parts[1]); err != nil {
		return ""
	}
	return path
}

func writeManagedFFmpegMarker(targetDir, executablePath string) error {
	sum, err := fileSHA256(executablePath)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(targetDir, managedFFmpegMarker), []byte(managedFFmpegSHA256+"\n"+sum+"\n"), 0o644)
}

func verifyFileSHA256(path, expected string) error {
	got, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("SHA-256 不匹配: 得到 %s", got)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func extractFFmpegExe(zipPath string, localPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开 ffmpeg 压缩包失败: %w", err)
	}
	defer zr.Close()

	for _, file := range zr.File {
		if strings.ToLower(filepath.Base(file.Name)) != managedFFmpegName() {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("拒绝解压符号链接形式的 ffmpeg")
		}
		src, err := file.Open()
		if err != nil {
			return fmt.Errorf("解压 ffmpeg 失败: %w", err)
		}
		defer src.Close()

		dst, err := os.CreateTemp(filepath.Dir(localPath), ".ffmpeg-install-*")
		if err != nil {
			return fmt.Errorf("写入 ffmpeg 失败: %w", err)
		}
		defer os.Remove(dst.Name())
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			return fmt.Errorf("解压 ffmpeg 失败: %w", err)
		}
		if err := dst.Close(); err != nil {
			return fmt.Errorf("写入 ffmpeg 失败: %w", err)
		}
		if err := os.Chmod(dst.Name(), 0o755); err != nil {
			return err
		}
		return os.Rename(dst.Name(), localPath)
	}
	return fmt.Errorf("压缩包中未找到 %s", managedFFmpegName())
}

func canAutoInstallFFmpeg(goos string) bool {
	return goos == "windows" || goos == "darwin"
}

type ffmpegProgressReader struct {
	Reader     io.Reader
	Total      int64
	Current    int64
	OnProgress func(int64)
}

func (p *ffmpegProgressReader) Read(b []byte) (int, error) {
	n, err := p.Reader.Read(b)
	p.Current += int64(n)
	if n > 0 && p.OnProgress != nil {
		p.OnProgress(p.Current)
	}
	return n, err
}
