package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"wx_channel/internal/config"
	"wx_channel/internal/interceptor"
	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
	"wx_channel/pkg/certificate"
	"wx_channel/pkg/system"
)

type Runtime struct {
	Config      *config.Config
	Settings    config.AppSettings
	Store       *miniprogram.Store
	Downloader  *minidownload.Downloader
	proxyServer *interceptor.InterceptorServer
	cert        *certificate.CertFileAndKeyFile
	mu          sync.Mutex
	ffmpegMu    sync.Mutex
	progressMu  sync.RWMutex
	progress    minidownload.Progress
	started     bool
}

func NewRuntime(cfg *config.Config) *Runtime {
	settings := cfg.Settings()
	store := miniprogram.NewStore()
	downloader := minidownload.New(settings.DownloadDir)
	downloader.FFmpegPath = configuredFFmpegPath(settings.FFmpegPath)
	downloader.SegmentConcurrency = settings.SegmentConcurrency
	downloader.SegmentRetries = settings.SegmentRetries
	downloader.KeepSegments = settings.KeepSegments
	downloader.DownloadMode = settings.DownloadMode
	r := &Runtime{
		Config:     cfg,
		Settings:   settings,
		Store:      store,
		Downloader: downloader,
	}
	downloader.OnProgress = r.setProgress
	downloader.OnLiveResolved = r.setLiveResolved
	return r
}

func (r *Runtime) PreflightChecks() []string {
	var checks []string
	if err := os.MkdirAll(r.Settings.DownloadDir, 0o755); err != nil {
		checks = append(checks, "自检失败: 下载目录不可写: "+err.Error())
	} else {
		temp, err := os.CreateTemp(r.Settings.DownloadDir, ".wx-mini-video-write-test-*")
		if err != nil {
			checks = append(checks, "自检失败: 下载目录不可写: "+err.Error())
		} else {
			_ = temp.Close()
			_ = os.Remove(temp.Name())
			checks = append(checks, "自检通过: 下载目录可写")
		}
	}
	addr := fmt.Sprintf("%s:%d", r.Settings.ProxyHostname, r.Settings.ProxyPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		checks = append(checks, "自检失败: 代理端口不可用 "+addr+": "+err.Error())
	} else {
		_ = ln.Close()
		checks = append(checks, "自检通过: 代理端口可用 "+addr)
	}
	switch _, source := r.locateFFmpeg(); source {
	case ffmpegSourceConfigured:
		checks = append(checks, "自检通过: ffmpeg 可用（已配置路径）")
	case ffmpegSourceBundled:
		checks = append(checks, "自检通过: ffmpeg 可用（随包）")
	case ffmpegSourceManaged:
		checks = append(checks, "自检通过: ffmpeg 可用（已下载）")
	default:
		if configured := configuredFFmpegPath(r.Settings.FFmpegPath); configured != "" {
			checks = append(checks, "自检失败: 配置的 download.ffmpeg 不可用 "+configured)
		} else {
			checks = append(checks, "自检提示: 随包无 ffmpeg，首次下载 m3u8 时会联网准备受管 ffmpeg")
		}
	}
	return checks
}

func (r *Runtime) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return nil
	}
	if r.proxyServer != nil {
		if err := r.proxyServer.Stop(); err != nil {
			return err
		}
		r.proxyServer = nil
	}
	if r.cert == nil {
		cert, err := config.LoadCertFiles()
		if err != nil {
			return fmt.Errorf("准备根证书失败: %w", err)
		}
		r.cert = cert
	}
	certPath, err := r.exportRootCert()
	if err != nil {
		return fmt.Errorf("导出公开根证书失败: %w", err)
	}
	interceptorCfg := interceptor.NewInterceptorSettings(r.Config, r.Settings, r.Store)
	server := interceptor.NewInterceptorServer(interceptorCfg, r.cert)
	r.proxyServer = server
	server.Interceptor.AddPostPlugin(interceptor.CreateMiniProgramInterceptorPlugin(r.Settings.Target, r.Store, r.Settings.DownloadDir, !r.Settings.HideUnattributed))
	if err := server.Start(); err != nil {
		if certPath != "" {
			return fmt.Errorf("%w；如证书自动安装失败，请按系统授权提示处理，或手动安装公开证书文件: %s", err, certPath)
		}
		return err
	}
	r.started = true
	return nil
}

func (r *Runtime) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.proxyServer == nil {
		return nil
	}
	err := r.proxyServer.Stop()
	r.started = false
	if err == nil {
		r.proxyServer = nil
	}
	return err
}

func (r *Runtime) CheckNetwork() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.proxyServer == nil {
		return nil
	}
	return r.proxyServer.Interceptor.CheckNetwork()
}

func (r *Runtime) Candidates() []miniprogram.Candidate {
	return r.Store.List(r.Settings.Target.AppID)
}

func (r *Runtime) ClearCandidates() {
	r.Store.Clear(r.Settings.Target.AppID)
}

func (r *Runtime) ClearProgress() {
	r.progressMu.Lock()
	r.progress = minidownload.Progress{}
	r.progressMu.Unlock()
}

func (r *Runtime) Download(ctx context.Context, candidate miniprogram.Candidate, preferredName string) (string, error) {
	if candidate.ID != "" {
		if latest, ok := r.Store.Get(candidate.ID); ok {
			candidate = latest
		}
	}
	if needsFFmpeg(candidate) && !r.HasFFmpeg() {
		r.setProgress(minidownload.Progress{
			ID:     candidate.ID,
			Status: "prepare-ffmpeg",
		})
		if err := r.EnsureFFmpeg(ctx, nil); err != nil {
			return "", err
		}
	}
	return r.Downloader.Download(ctx, candidate, preferredName, r.Settings.Target.AppID)
}

func (r *Runtime) DownloadProgress() minidownload.Progress {
	r.progressMu.RLock()
	defer r.progressMu.RUnlock()
	return r.progress
}

func (r *Runtime) setProgress(progress minidownload.Progress) {
	r.progressMu.Lock()
	r.progress = progress
	r.progressMu.Unlock()
}

// setLiveResolved 把下载阶段确认的直播状态回写到候选存储，
// 供界面按候选身份维持选择：疑似直播确认是有限文件后归回视频，反之亦然。
func (r *Runtime) setLiveResolved(candidateID string, live bool) {
	if r.Store != nil {
		r.Store.SetLive(candidateID, live)
	}
}

// downloadDirForTarget 返回应打开的目录：目标目录存在时打开目标目录，否则回落下载根目录。
func (r *Runtime) downloadDirForTarget(targetAppID string) string {
	root := r.Settings.DownloadDir
	appID := strings.TrimSpace(targetAppID)
	if appID == "" {
		return root
	}
	targetDir := filepath.Join(root, miniprogram.SanitizeAppIDDir(appID))
	if info, err := os.Stat(targetDir); err == nil && info.IsDir() {
		return targetDir
	}
	return root
}

func (r *Runtime) OpenDownloadDir(targetAppID string) error {
	return system.Open(r.downloadDirForTarget(targetAppID))
}

func (r *Runtime) SetTarget(target miniprogram.Target) {
	target.AppID = strings.TrimSpace(target.AppID)
	r.mu.Lock()
	r.Settings.Target = target
	if r.Store != nil {
		r.Store.SetTarget(target)
	}
	r.ClearProgress()
	r.mu.Unlock()
}

const (
	ffmpegSourceConfigured = "configured"
	ffmpegSourceBundled    = "bundled"
	ffmpegSourceManaged    = "managed"
)

// bundledFFmpegPath 与 managedFFmpegPath 是测试注入点，默认使用真实随包/受管位置。
var (
	bundledFFmpegPath = minidownload.BundledFFmpegPath
	managedFFmpegPath = minidownload.ManagedFFmpegPath
)

func (r *Runtime) HasFFmpeg() bool {
	path, _ := r.locateFFmpeg()
	if path == "" {
		return false
	}
	r.Downloader.FFmpegPath = path
	return true
}

// locateFFmpeg 按 显式配置 > 随包 ffmpeg > 受管下载 的优先级定位 ffmpeg，不修改下载器状态。
func (r *Runtime) locateFFmpeg() (string, string) {
	if path := configuredFFmpegPath(r.Settings.FFmpegPath); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, ffmpegSourceConfigured
		}
		return "", ""
	}
	if path := bundledFFmpegPath(); path != "" {
		return path, ffmpegSourceBundled
	}
	if path := managedFFmpegPath(r.Settings.DownloadDir); path != "" {
		return path, ffmpegSourceManaged
	}
	return "", ""
}

func (r *Runtime) EnsureFFmpeg(ctx context.Context, onProgress func(minidownload.FFmpegState)) error {
	r.ffmpegMu.Lock()
	defer r.ffmpegMu.Unlock()
	if path := configuredFFmpegPath(r.Settings.FFmpegPath); path != "" {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("配置的 download.ffmpeg 不可用: %s；请修正该路径或留空以使用随包或受管 ffmpeg", path)
		}
		if onProgress != nil {
			onProgress(minidownload.FFmpegState{State: "found", Path: path})
		}
		r.mu.Lock()
		r.Downloader.FFmpegPath = path
		r.mu.Unlock()
		return nil
	}
	if path := bundledFFmpegPath(); path != "" {
		if onProgress != nil {
			onProgress(minidownload.FFmpegState{State: "found", Path: path})
		}
		r.mu.Lock()
		r.Downloader.FFmpegPath = path
		r.mu.Unlock()
		return nil
	}
	dir := r.Settings.DownloadDir
	if dir == "" {
		dir = "."
	}
	path, err := minidownload.EnsureFFmpeg(ctx, dir, onProgress)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.Downloader.FFmpegPath = path
	r.mu.Unlock()
	return nil
}

func configuredFFmpegPath(configured string) string {
	path := strings.TrimSpace(configured)
	if strings.EqualFold(path, "ffmpeg") {
		return ""
	}
	return path
}

func needsFFmpeg(candidate miniprogram.Candidate) bool {
	return miniprogram.IsM3U8(candidate)
}

func (r *Runtime) ProxyAddr() string {
	return fmt.Sprintf("%s:%d", r.Settings.ProxyHostname, r.Settings.ProxyPort)
}

func (r *Runtime) exportRootCert() (string, error) {
	if len(r.cert.Cert) == 0 || r.Settings.DownloadDir == "" {
		return "", fmt.Errorf("证书或下载目录为空")
	}
	if err := os.MkdirAll(r.Settings.DownloadDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(r.Settings.DownloadDir, "WxMiniVideoRoot.cer")
	if err := os.WriteFile(path, r.cert.Cert, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
