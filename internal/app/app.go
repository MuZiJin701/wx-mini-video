package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
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
		cert:       config.LoadCertFiles(),
	}
	downloader.OnProgress = r.setProgress
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
	if r.HasFFmpeg() {
		checks = append(checks, "自检通过: ffmpeg 可用")
	} else {
		checks = append(checks, "自检提示: 未检测到 ffmpeg，Windows 下会自动下载")
	}
	return checks
}

func (r *Runtime) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return nil
	}
	if strings.TrimSpace(r.Settings.Target.AppID) == "" {
		return fmt.Errorf("请先输入小程序 AppID")
	}
	addr := fmt.Sprintf("%s:%d", r.Settings.ProxyHostname, r.Settings.ProxyPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("代理端口不可用 %s: %w", addr, err)
	}
	_ = ln.Close()
	interceptorCfg := interceptor.NewInterceptorSettings(r.Config, r.Settings, r.Store)
	server := interceptor.NewInterceptorServer(interceptorCfg, r.cert)
	server.Interceptor.AddPostPlugin(interceptor.CreateMiniProgramInterceptorPlugin(r.Settings.Target, r.Store, r.Settings.DownloadDir))
	certPath := r.exportRootCert()
	if err := server.Start(); err != nil {
		if certPath != "" {
			return fmt.Errorf("%w；如证书自动安装失败，请用管理员权限重新运行，或手动安装证书文件: %s", err, certPath)
		}
		return err
	}
	r.proxyServer = server
	r.started = true
	return nil
}

func (r *Runtime) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.started || r.proxyServer == nil {
		return nil
	}
	err := r.proxyServer.Stop()
	r.started = false
	return err
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
	return r.Downloader.Download(ctx, candidate, preferredName)
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

func (r *Runtime) OpenDownloadDir() error {
	return system.Open(r.Settings.DownloadDir)
}

func (r *Runtime) SetTarget(target miniprogram.Target) {
	target.AppID = strings.TrimSpace(target.AppID)
	target.Name = strings.TrimSpace(target.Name)
	r.mu.Lock()
	r.Settings.Target = target
	r.mu.Unlock()
}

func (r *Runtime) HasFFmpeg() bool {
	path := configuredFFmpegPath(r.Downloader.FFmpegPath)
	_, err := exec.LookPath(path)
	if err == nil {
		r.Downloader.FFmpegPath = path
	}
	return err == nil
}

func (r *Runtime) EnsureFFmpeg(ctx context.Context, onProgress func(minidownload.FFmpegState)) error {
	r.ffmpegMu.Lock()
	defer r.ffmpegMu.Unlock()
	dir := r.Settings.DownloadDir
	if dir == "" {
		dir = "."
	}
	if path := configuredFFmpegPath(r.Downloader.FFmpegPath); path != "" {
		if _, err := exec.LookPath(path); err == nil {
			if onProgress != nil {
				onProgress(minidownload.FFmpegState{State: "found", Path: path})
			}
			r.Downloader.FFmpegPath = path
			return nil
		}
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
	if configured != "" {
		return configured
	}
	return "ffmpeg"
}

func needsFFmpeg(candidate miniprogram.Candidate) bool {
	if candidate.Kind == "m3u8" {
		return true
	}
	kind, _ := miniprogram.Classify("", candidate.URL)
	return kind == "m3u8"
}

func (r *Runtime) ProxyAddr() string {
	return fmt.Sprintf("%s:%d", r.Settings.ProxyHostname, r.Settings.ProxyPort)
}

func (r *Runtime) exportRootCert() string {
	if len(r.cert.Cert) == 0 || r.Settings.DownloadDir == "" {
		return ""
	}
	if err := os.MkdirAll(r.Settings.DownloadDir, 0o755); err != nil {
		return ""
	}
	path := filepath.Join(r.Settings.DownloadDir, "WxMiniVideoRoot.cer")
	if err := os.WriteFile(path, r.cert.Cert, 0o644); err != nil {
		return ""
	}
	return path
}
