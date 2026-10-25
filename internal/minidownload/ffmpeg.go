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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	managedFFmpegMarker    = ".wx-mini-video-ffmpeg.sha256"
	ffmpegDownloadAttempts = 3
	// ffmpegSourceTimeout 限制单个候选源的整体下载时间，慢速或卡住的源会被整体放弃并尝试下一个。
	// 109 MB 的 gyan 压缩包在 1 Mbps 链路上约需 15 分钟，留出余量避免正常慢速网络被误判。
	ffmpegSourceTimeout = 20 * time.Minute
	// ffmpegHeaderTimeout 限制等待响应头的时间，连接被挂起时能更快切换备用源。
	ffmpegHeaderTimeout = 30 * time.Second
	// ffmpegDownloadUserAgent 标识下载请求，部分 CDN 会拒绝空或默认 User-Agent 的请求。
	ffmpegDownloadUserAgent = "wx-mini-video-ffmpeg-downloader"
)

// FFmpegSource 是一个固定版本、固定 SHA-256 的受管 ffmpeg 下载源。
type FFmpegSource struct {
	URL    string
	SHA256 string
}

var (
	managedFFmpegSources = ffmpegSourcesForPlatform(runtime.GOOS, runtime.GOARCH)
	ffmpegHTTPClient     = newFFmpegHTTPClient()
)

// FFmpegSourcesForPlatform 返回指定平台的受管 ffmpeg 下载源，顺序即尝试顺序。
func FFmpegSourcesForPlatform(goos, arch string) []FFmpegSource {
	sources := ffmpegSourcesForPlatform(goos, arch)
	out := make([]FFmpegSource, len(sources))
	copy(out, sources)
	return out
}

// ffmpegSourcesForPlatform 返回按优先级排列的候选源；旧固定地址失效时可切换到备用源。
func ffmpegSourcesForPlatform(goos, arch string) []FFmpegSource {
	if goos == "windows" {
		return []FFmpegSource{
			{
				URL:    "https://www.gyan.dev/ffmpeg/builds/packages/ffmpeg-9.0.2-essentials_build.zip",
				SHA256: "60f467265b1e312373dbcd92200c2618a74850f98d3d078e94296bb3fa2047ba",
			},
			{
				URL:    "https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-09-30-13-08/ffmpeg-n8.1.3-9-g29e619e767-win64-lgpl-8.1.zip",
				SHA256: "4a7642b2264c03e8a0ce8a3825b933ee5580656f45695a086fe7e294045ffc0a",
			},
		}
	}
	if goos == "darwin" {
		switch arch {
		case "amd64":
			return []FFmpegSource{{
				URL:    "https://ffmpeg.martin-riedl.de/download/macos/amd64/1789931006_9.0.2/ffmpeg.zip",
				SHA256: "7c6b4125b191cbf773832dc51f424cf2b6bb7da43007d1e066f95909e47cacd4",
			}}
		case "arm64":
			return []FFmpegSource{{
				URL:    "https://ffmpeg.martin-riedl.de/download/macos/arm64/1789931890_9.0.2/ffmpeg.zip",
				SHA256: "c8ed4c4e6978a03c485edbfe4e0a5dc2380f8a30bba5150531b31b094492d924",
			}}
		}
	}
	return nil
}

func managedFFmpegName() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}

// BundledFFmpegPath 返回随包 ffmpeg 的路径；未随包提供或不可执行时返回空字符串。
// 只根据程序自身位置定位，绝不使用配置目录或下载目录：
//   - Windows：<程序目录>\ffmpeg\ffmpeg.exe
//   - macOS：<包根>/ffmpeg/ffmpeg-amd64 或 ffmpeg-arm64；程序位于
//     *.app/Contents/MacOS 时包根为 .app 的父目录，否则为程序所在目录。
func BundledFFmpegPath() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return ""
	}
	return checkedBundledFFmpeg(bundledFFmpegPath(filepath.Dir(absolute), runtime.GOOS, runtime.GOARCH))
}

var bundledFFmpegProbeCache = struct {
	sync.Mutex
	path     string
	size     int64
	modified time.Time
	usable   bool
}{}

// 同一文件只探测一次；替换或修复随包文件后重新检查，避免渲染时反复启动进程。
func checkedBundledFFmpeg(path string) string {
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || !isExecutableFile(path, runtime.GOOS) {
		return ""
	}
	cache := &bundledFFmpegProbeCache
	cache.Lock()
	defer cache.Unlock()
	if cache.path != path || cache.size != info.Size() || !cache.modified.Equal(info.ModTime()) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, path, "-version")
		command.WaitDelay = time.Second
		output, err := command.Output()
		cache.path, cache.size, cache.modified = path, info.Size(), info.ModTime()
		cache.usable = err == nil && strings.HasPrefix(strings.TrimSpace(string(output)), "ffmpeg version ")
	}
	if !cache.usable {
		return ""
	}
	return path
}

// bundledFFmpegPath 按包布局计算随包 ffmpeg 路径并做轻量可用性检查。
func bundledFFmpegPath(exeDir, goos, arch string) string {
	candidate := bundledFFmpegCandidate(exeDir, goos, arch)
	if candidate == "" || !isExecutableFile(candidate, goos) {
		return ""
	}
	return candidate
}

// bundledFFmpegCandidate 返回指定布局下随包 ffmpeg 的预期路径，不检查文件是否存在。
func bundledFFmpegCandidate(exeDir, goos, arch string) string {
	switch goos {
	case "windows":
		return filepath.Join(exeDir, "ffmpeg", "ffmpeg.exe")
	case "darwin":
		name := ""
		switch arch {
		case "amd64":
			name = "ffmpeg-amd64"
		case "arm64":
			name = "ffmpeg-arm64"
		default:
			return ""
		}
		return filepath.Join(macosPackageRoot(exeDir), "ffmpeg", name)
	}
	return ""
}

// macosPackageRoot 把 <包根>/wx-mini-video.app/Contents/MacOS 还原为 <包根>；
// 其他布局按程序所在目录处理。
func macosPackageRoot(exeDir string) string {
	clean := filepath.Clean(exeDir)
	contentsDir := filepath.Dir(clean)
	appDir := filepath.Dir(contentsDir)
	if filepath.Base(clean) == "MacOS" && filepath.Base(contentsDir) == "Contents" && strings.HasSuffix(filepath.Base(appDir), ".app") {
		return filepath.Dir(appDir)
	}
	return exeDir
}

// isExecutableFile 做“存在 + 可执行”的轻量检查，不读取文件内容。
// Windows 不以权限位表达可执行，存在普通文件即可视为可用。
func isExecutableFile(path, goos string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if goos == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
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

	if !canAutoInstallFFmpeg(runtime.GOOS) || len(managedFFmpegSources) == 0 {
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
	source, err := downloadFFmpegArchive(ctx, zipPath, emit)
	if err != nil {
		return "", managedFFmpegError(err)
	}
	defer os.Remove(zipPath)

	emit(FFmpegState{State: "extracting"})
	if err := extractFFmpegExe(zipPath, localPath); err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return "", managedFFmpegError(err)
	}
	if err := writeManagedFFmpegMarker(targetDir, localPath, source.SHA256); err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return "", managedFFmpegError(fmt.Errorf("写入 ffmpeg 校验标记失败: %w", err))
	}

	emit(FFmpegState{State: "done", Path: localPath})
	return localPath, nil
}

// downloadFFmpegArchive 按候选源顺序下载并校验压缩包，单个源失败时切换到下一个源。
func downloadFFmpegArchive(ctx context.Context, zipPath string, emit func(FFmpegState)) (FFmpegSource, error) {
	var lastErr error
	for _, source := range managedFFmpegSources {
		if err := downloadFFmpegSource(ctx, zipPath, source, emit); err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return FFmpegSource{}, ctx.Err()
			}
			continue
		}
		return source, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("没有可用的 ffmpeg 下载源")
	}
	return FFmpegSource{}, fmt.Errorf("所有 ffmpeg 下载源均失败: %w", lastErr)
}

// downloadFFmpegSource 下载单个固定源，临时故障在同一源内有限重试。
func downloadFFmpegSource(ctx context.Context, zipPath string, source FFmpegSource, emit func(FFmpegState)) error {
	ctx, cancel := context.WithTimeout(ctx, ffmpegSourceTimeout)
	defer cancel()
	var lastErr error
	for attempt := 0; attempt < ffmpegDownloadAttempts; attempt++ {
		retryable, err := downloadFFmpegArchiveOnce(ctx, zipPath, source, emit)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable {
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

func downloadFFmpegArchiveOnce(ctx context.Context, zipPath string, source FFmpegSource, emit func(FFmpegState)) (retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return false, fmt.Errorf("创建 ffmpeg 下载请求失败: %w", err)
	}
	req.Header.Set("User-Agent", ffmpegDownloadUserAgent)
	resp, err := ffmpegHTTPClient.Do(req)
	if err != nil {
		emit(FFmpegState{State: "error", Error: err})
		return true, fmt.Errorf("下载 ffmpeg 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("下载 ffmpeg 失败: HTTP %s", resp.Status)
		emit(FFmpegState{State: "error", Error: err})
		return retryableFFmpegStatus(resp.StatusCode), err
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
	if err := verifyFileSHA256(zipPath, source.SHA256); err != nil {
		_ = os.Remove(zipPath)
		emit(FFmpegState{State: "error", Error: err})
		return false, fmt.Errorf("下载的 ffmpeg 校验失败: %w", err)
	}
	return false, nil
}

// retryableFFmpegStatus 判断 HTTP 状态是否值得在同一源上重试。
func retryableFFmpegStatus(status int) bool {
	return status >= 500 || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests
}

func newFFmpegHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = ffmpegHeaderTimeout
	return &http.Client{Transport: transport}
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
	if err != nil || len(parts) != 2 || !isPinnedFFmpegArchive(parts[0]) {
		return ""
	}
	if err := verifyFileSHA256(path, parts[1]); err != nil {
		return ""
	}
	return path
}

// isPinnedFFmpegArchive 判断标记中的压缩包摘要是否属于当前固定源，旧版本标记会自动失效。
func isPinnedFFmpegArchive(sum string) bool {
	for _, source := range managedFFmpegSources {
		if strings.EqualFold(source.SHA256, sum) {
			return true
		}
	}
	return false
}

func writeManagedFFmpegMarker(targetDir, executablePath, archiveSHA string) error {
	sum, err := fileSHA256(executablePath)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(targetDir, managedFFmpegMarker), []byte(archiveSHA+"\n"+sum+"\n"), 0o644)
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
		if !strings.EqualFold(filepath.Base(file.Name), managedFFmpegName()) {
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
