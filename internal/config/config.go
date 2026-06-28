package config

import (
	"bytes"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/viper"

	"wx_channel/internal/miniprogram"
	"wx_channel/pkg/certificate"
)

type Config struct {
	RootDir  string
	Filename string
	FullPath string
	Existing bool
	Error    error
	Version  string
	Mode     string
}

type AppSettings struct {
	DownloadDir        string
	FFmpegPath         string
	SegmentConcurrency int
	SegmentRetries     int
	KeepSegments       bool
	DownloadMode       string
	ProxySystem        bool
	ProxyHostname      string
	ProxyPort          int
	ProxySkipRootCert  bool
	ProxyUpstream      string
	Target             miniprogram.Target
}

const EnvConfigPath = "WX_MINI_VIDEO_CONFIG"

func New(ver string, mode string) *Config {
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	baseDir := exeDir
	filename := "wx-mini-video.yaml"
	configPath := ""
	existing := false

	if envPath := strings.TrimSpace(os.Getenv(EnvConfigPath)); envPath != "" {
		configPath = envPath
		if abs, err := filepath.Abs(envPath); err == nil {
			configPath = abs
		}
		baseDir = filepath.Dir(configPath)
		filename = filepath.Base(configPath)
		if _, err := os.Stat(configPath); err == nil {
			existing = true
		}
		viper.SetConfigFile(configPath)
		return &Config{RootDir: baseDir, Filename: filename, FullPath: configPath, Existing: existing, Version: ver, Mode: mode}
	}

	for _, dir := range configCandidates(exeDir) {
		path := filepath.Join(dir, filename)
		if _, err := os.Stat(path); err == nil {
			baseDir = dir
			configPath = path
			existing = true
			break
		}
	}
	viper.SetConfigFile(configPath)
	return &Config{RootDir: baseDir, Filename: filename, FullPath: configPath, Existing: existing, Version: ver, Mode: mode}
}

func configCandidates(exeDir string) []string {
	candidates := []string{exeDir}
	if _, callerFile, _, ok := runtime.Caller(1); ok {
		candidates = append(candidates, filepath.Dir(callerFile))
	}
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Dir(filepath.Dir(thisFile)))
	}
	return candidates
}

func (c *Config) LoadConfig() error {
	Registry = nil
	registerDefaults()
	if c.Existing {
		if err := viper.ReadInConfig(); err != nil {
			var nf viper.ConfigFileNotFoundError
			if !(errors.As(err, &nf) || errors.Is(err, os.ErrNotExist)) {
				c.Error = err
				return err
			}
		}
	}
	return nil
}

func registerDefaults() {
	Register(ConfigItem{Key: "download.dir", Type: ConfigTypeString, Default: "", Title: "下载目录", Group: "Download"})
	Register(ConfigItem{Key: "download.ffmpeg", Type: ConfigTypeString, Default: "ffmpeg", Title: "FFmpeg 路径", Group: "Download"})
	Register(ConfigItem{Key: "download.segmentConcurrency", Type: ConfigTypeInt, Default: 6, Title: "分片并发数", Group: "Download"})
	Register(ConfigItem{Key: "download.segmentRetries", Type: ConfigTypeInt, Default: 3, Title: "分片重试次数", Group: "Download"})
	Register(ConfigItem{Key: "download.keepSegments", Type: ConfigTypeBool, Default: false, Title: "保留分片缓存", Group: "Download"})
	Register(ConfigItem{Key: "download.mode", Type: ConfigTypeString, Default: "auto", Title: "m3u8 下载模式", Group: "Download"})
	Register(ConfigItem{Key: "proxy.system", Type: ConfigTypeBool, Default: true, Title: "设置系统代理", Group: "Proxy"})
	Register(ConfigItem{Key: "proxy.hostname", Type: ConfigTypeString, Default: "127.0.0.1", Title: "代理主机", Group: "Proxy"})
	Register(ConfigItem{Key: "proxy.port", Type: ConfigTypeInt, Default: 2023, Title: "代理端口", Group: "Proxy"})
	Register(ConfigItem{Key: "proxy.skipInstallRootCert", Type: ConfigTypeBool, Default: false, Title: "跳过证书安装", Group: "Proxy"})
	Register(ConfigItem{Key: "proxy.upstreamProxy", Type: ConfigTypeString, Default: "", Title: "上游代理", Group: "Proxy"})
	Register(ConfigItem{Key: "target.appID", Type: ConfigTypeString, Default: "", Title: "小程序 AppID", Group: "Target"})
	Register(ConfigItem{Key: "target.name", Type: ConfigTypeString, Default: "", Title: "小程序名称", Group: "Target"})
	Register(ConfigItem{Key: "cert.file", Type: ConfigTypeString, Default: "", Title: "证书文件", Group: "Cert"})
	Register(ConfigItem{Key: "cert.key", Type: ConfigTypeString, Default: "", Title: "私钥文件", Group: "Cert"})
	Register(ConfigItem{Key: "cert.name", Type: ConfigTypeString, Default: "Echo", Title: "证书名称", Group: "Cert"})
}

func (c *Config) Settings() AppSettings {
	downloadDir := resolveDownloadDir(viper.GetString("download.dir"))
	return AppSettings{
		DownloadDir:        downloadDir,
		FFmpegPath:         viper.GetString("download.ffmpeg"),
		SegmentConcurrency: viper.GetInt("download.segmentConcurrency"),
		SegmentRetries:     viper.GetInt("download.segmentRetries"),
		KeepSegments:       viper.GetBool("download.keepSegments"),
		DownloadMode:       viper.GetString("download.mode"),
		ProxySystem:        viper.GetBool("proxy.system"),
		ProxyHostname:      viper.GetString("proxy.hostname"),
		ProxyPort:          viper.GetInt("proxy.port"),
		ProxySkipRootCert:  viper.GetBool("proxy.skipInstallRootCert"),
		ProxyUpstream:      viper.GetString("proxy.upstreamProxy"),
		Target: miniprogram.Target{
			Name:  strings.TrimSpace(viper.GetString("target.name")),
			AppID: strings.TrimSpace(viper.GetString("target.appID")),
		},
	}
}

func resolveDownloadDir(value string) string {
	if value == "" {
		if wd, err := os.Getwd(); err == nil {
			return filepath.Join(wd, "downloads")
		}
		return "downloads"
	}
	if strings.HasPrefix(value, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(value, "~"))
		}
	}
	return value
}

func LoadCertFiles() *certificate.CertFileAndKeyFile {
	cert := certificate.DefaultCertFiles
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".mitmproxy"))
	}
	if runtime.GOOS == "windows" {
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			dirs = append(dirs, filepath.Join(appdata, "mitmproxy"))
		}
	}
	for _, dir := range dirs {
		certPath := filepath.Join(dir, "mitmproxy-ca-cert.pem")
		keyPath := filepath.Join(dir, "mitmproxy-ca.pem")
		if certBytes, err1 := os.ReadFile(certPath); err1 == nil {
			if keyBytes, err2 := os.ReadFile(keyPath); err2 == nil {
				return &certificate.CertFileAndKeyFile{Name: "mitmproxy", Cert: certBytes, PrivateKey: keyBytes}
			}
		}
		if keyBytes, err := os.ReadFile(keyPath); err == nil {
			if loaded := splitMitmproxyKey(keyBytes); loaded != nil {
				return loaded
			}
		}
	}
	certPath := viper.GetString("cert.file")
	keyPath := viper.GetString("cert.key")
	if certPath != "" && keyPath != "" {
		if certBytes, err1 := os.ReadFile(certPath); err1 == nil {
			if keyBytes, err2 := os.ReadFile(keyPath); err2 == nil {
				cert = &certificate.CertFileAndKeyFile{Name: viper.GetString("cert.name"), Cert: certBytes, PrivateKey: keyBytes}
			}
		}
	}
	return cert
}

func splitMitmproxyKey(keyBytes []byte) *certificate.CertFileAndKeyFile {
	rest := keyBytes
	var certBlocks [][]byte
	var keyBlock []byte
	for {
		block, rem := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = rem
		if block.Type == "CERTIFICATE" {
			if enc := pem.EncodeToMemory(block); enc != nil {
				certBlocks = append(certBlocks, enc)
			}
		} else if strings.Contains(block.Type, "PRIVATE KEY") {
			keyBlock = pem.EncodeToMemory(block)
		}
	}
	if len(certBlocks) == 0 || len(keyBlock) == 0 {
		return nil
	}
	return &certificate.CertFileAndKeyFile{Name: "mitmproxy", Cert: bytes.Join(certBlocks, []byte("")), PrivateKey: keyBlock}
}

func (c *Config) GetDebugInfo() map[string]string {
	return map[string]string{
		"base_dir":      c.RootDir,
		"config_path":   c.FullPath,
		"config_exists": fmt.Sprintf("%v", c.Existing),
	}
}
