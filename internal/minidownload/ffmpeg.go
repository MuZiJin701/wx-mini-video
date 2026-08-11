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
)

const (
	managedFFmpegURL    = "https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-08-11-13-11/ffmpeg-n8.1.2-34-g9b6c8969e0-win64-gpl-8.1.zip"
	managedFFmpegSHA256 = "05eedc113542be39af5d0f78f0b1093bafb89c98cecf25b77e8644670293107f"
	managedFFmpegMarker = ".wx-mini-video-ffmpeg.sha256"
)

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

	if !canAutoInstallFFmpeg(runtime.GOOS) {
		err := fmt.Errorf("未找到 ffmpeg，请先安装 ffmpeg 或在配置中设置 download.ffmpeg")
		emit(FFmpegState{State: "error", Error: err})
		return "", err
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	localPath := filepath.Join(targetDir, "ffmpeg.exe")
	zipPath := filepath.Join(targetDir, "ffmpeg_temp.zip")
	if err := downloadFFmpegArchive(ctx, zipPath, emit); err != nil {
		return "", err
	}
	defer os.Remove(zipPath)

	emit(FFmpegState{State: "extracting"})
	if err := extractFFmpegExe(zipPath, localPath); err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return "", err
	}
	if err := os.WriteFile(filepath.Join(targetDir, managedFFmpegMarker), []byte(managedFFmpegSHA256+"\n"), 0o644); err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return "", fmt.Errorf("写入 ffmpeg 校验标记失败: %w", err)
	}

	emit(FFmpegState{State: "done", Path: localPath})
	return localPath, nil
}

func downloadFFmpegArchive(ctx context.Context, zipPath string, emit func(FFmpegState)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, managedFFmpegURL, nil)
	if err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return fmt.Errorf("创建 ffmpeg 下载请求失败: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return fmt.Errorf("下载 ffmpeg 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("下载 ffmpeg 失败: HTTP %s", resp.Status)
		emit(FFmpegState{State: "error", Error: err})
		return err
	}

	file, err := os.Create(zipPath)
	if err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return fmt.Errorf("创建临时文件失败: %w", err)
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
		os.Remove(zipPath)
		emit(FFmpegState{State: "error", Error: err})
		return fmt.Errorf("下载 ffmpeg 失败: %w", err)
	}
	if err := file.Close(); err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return fmt.Errorf("写入 ffmpeg 失败: %w", err)
	}
	if err := verifyFileSHA256(zipPath, managedFFmpegSHA256); err != nil {
		_ = os.Remove(zipPath)
		emit(FFmpegState{State: "error", Error: err})
		return fmt.Errorf("下载的 ffmpeg 校验失败: %w；请检查网络或配置 download.ffmpeg 为本地 ffmpeg.exe", err)
	}
	return nil
}

func ManagedFFmpegPath(targetDir string) string {
	path := filepath.Join(targetDir, "ffmpeg.exe")
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	marker, err := os.ReadFile(filepath.Join(targetDir, managedFFmpegMarker))
	if err != nil || strings.TrimSpace(string(marker)) != managedFFmpegSHA256 {
		return ""
	}
	return path
}

func verifyFileSHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, expected) {
		return fmt.Errorf("SHA-256 不匹配: 得到 %s", got)
	}
	return nil
}

func extractFFmpegExe(zipPath string, localPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开 ffmpeg 压缩包失败: %w", err)
	}
	defer zr.Close()

	for _, file := range zr.File {
		if strings.ToLower(filepath.Base(file.Name)) != "ffmpeg.exe" {
			continue
		}
		src, err := file.Open()
		if err != nil {
			return fmt.Errorf("解压 ffmpeg 失败: %w", err)
		}
		defer src.Close()

		dst, err := os.Create(localPath)
		if err != nil {
			return fmt.Errorf("写入 ffmpeg 失败: %w", err)
		}
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			os.Remove(localPath)
			return fmt.Errorf("解压 ffmpeg 失败: %w", err)
		}
		if err := dst.Close(); err != nil {
			os.Remove(localPath)
			return fmt.Errorf("写入 ffmpeg 失败: %w", err)
		}
		return nil
	}
	return fmt.Errorf("压缩包中未找到 ffmpeg.exe")
}

func canAutoInstallFFmpeg(goos string) bool {
	return goos == "windows"
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
