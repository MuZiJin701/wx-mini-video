package interceptor

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/ltaoo/echo"
	"github.com/rs/zerolog"

	"wx_channel/internal/interceptor/proxy"
	"wx_channel/pkg/certificate"
	"wx_channel/pkg/system"
)

type Interceptor struct {
	Version     string
	Settings    *InterceptorConfig
	Cert        *certificate.CertFileAndKeyFile
	proxy       proxy.InnerProxy
	PostPlugins []interface{}
	log         *zerolog.Logger
	proxyState  *system.ProxySnapshot
}

func NewInterceptor(cfg *InterceptorConfig, cert *certificate.CertFileAndKeyFile) *Interceptor {
	log := zerolog.New(io.Discard).With().Timestamp().Str("component", "interceptor").Str("version", cfg.Version).Logger()
	return &Interceptor{
		Version:  cfg.Version,
		Settings: cfg,
		Cert:     cert,
		log:      &log,
	}
}

func (c *Interceptor) Start() error {
	echo.SetLogEnabled(false)
	client, err := proxy.NewProxy(c.Cert.Cert, c.Cert.PrivateKey, c.Settings.ProxyUpstreamProxy)
	if err != nil {
		return err
	}
	for _, plugin := range c.PostPlugins {
		client.AddPlugin(plugin)
	}
	c.proxy = client
	if !c.Settings.ProxySkipInstallRootCert {
		existing, err := certificate.CheckHasCertificate(c.Cert.Name)
		if err != nil {
			return fmt.Errorf("检查证书失败: %v", err)
		}
		if !existing {
			if err := certificate.InstallCertificate(c.Cert.Cert); err != nil {
				return fmt.Errorf("安装证书失败: %v", err)
			}
		}
	}
	if c.Settings.ProxySetSystem {
		snapshot, err := system.CaptureProxySnapshot()
		if err != nil {
			return fmt.Errorf("读取原系统代理失败: %v", err)
		}
		if isOwnProxySnapshot(snapshot, c.Settings.ProxyServerHostname, strconv.Itoa(c.Settings.ProxyServerPort)) {
			snapshot = &system.ProxySnapshot{Enabled: false}
		}
		c.proxyState = snapshot
		if err := system.EnableProxy(system.ProxySettings{
			Hostname: c.Settings.ProxyServerHostname,
			Port:     strconv.Itoa(c.Settings.ProxyServerPort),
		}); err != nil {
			return fmt.Errorf("设置代理失败: %v", err)
		}
	}
	return client.Start(c.Settings.ProxyServerPort)
}

func (c *Interceptor) Stop() error {
	if c.Settings.ProxySetSystem {
		if err := system.RestoreProxySnapshot(c.proxyState); err != nil {
			return fmt.Errorf("恢复系统代理失败: %v", err)
		}
		c.proxyState = nil
	}
	if c.proxy != nil {
		if err := c.proxy.Close(); err != nil {
			return fmt.Errorf("关闭代理服务失败: %v", err)
		}
	}
	return nil
}

func isOwnProxySnapshot(snapshot *system.ProxySnapshot, hostname string, port string) bool {
	if snapshot == nil || !snapshot.Enabled || !snapshot.HasServer {
		return false
	}
	server := strings.ToLower(strings.TrimSpace(snapshot.Server))
	targets := []string{
		strings.ToLower(hostname + ":" + port),
		"localhost:" + port,
		"127.0.0.1:" + port,
	}
	for _, target := range targets {
		if server == target || strings.Contains(server, "="+target) || strings.Contains(server, ";"+target) {
			return true
		}
	}
	return false
}

func (c *Interceptor) AddPostPlugin(plugin interface{}) {
	c.PostPlugins = append(c.PostPlugins, plugin)
}

func (c *Interceptor) SetLog(writer io.Writer) {
	l := zerolog.New(writer).With().Timestamp().Str("component", "interceptor").Str("version", c.Version).Logger()
	c.log = &l
}

func (c *Interceptor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	isLocal := false
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		isLocal = true
	}
	if host == "localhost" || host == c.Settings.ProxyServerHostname {
		isLocal = true
	}
	if isLocal && r.URL.Path == "/cert" {
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.Header().Set("Content-Disposition", "attachment; filename=\"QimingRoot.cer\"")
		_, _ = w.Write(c.Cert.Cert)
		return
	}
	if isLocal && (r.URL.Path == "/" || r.URL.Path == "") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<html><head><title>wx-mini-video</title></head><body><h1>微信小程序视频代理运行中</h1><p><a href="/cert">下载根证书</a></p></body></html>`)
		return
	}
	c.proxy.ServeHTTP(w, r)
}
