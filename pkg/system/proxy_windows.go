//go:build windows

package system

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const internetSettingsKeyPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// proxyRegistryState 是从注册表读到的原始系统代理状态。
// 用 *Set 标志区分“值不存在”与“值为空/零”，缺失值不应被当成读取失败。
type proxyRegistryState struct {
	enableSet bool
	enable    uint64
	serverSet bool
	server    string
}

func enable_proxy(args ProxySettings) error {
	args = merge_default_settings(args)
	key, err := openInternetSettingsKeyForWrite()
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		return fmt.Errorf("设置系统代理开关失败: %w", err)
	}
	if err := key.SetStringValue("ProxyServer", fmt.Sprintf("%v:%v", args.Hostname, args.Port)); err != nil {
		return fmt.Errorf("设置系统代理地址失败: %w", err)
	}
	notify_proxy_changed()
	return nil
}

func disable_proxy(args ProxySettings) error {
	if err := set_proxy_enable(false); err != nil {
		return fmt.Errorf("设置系统代理开关失败: %w", err)
	}
	notify_proxy_changed()
	return nil
}

func capture_proxy_snapshot() (*ProxySnapshot, error) {
	return captureProxySnapshotAt(internetSettingsKeyPath)
}

func captureProxySnapshotAt(path string) (*ProxySnapshot, error) {
	state, err := readProxyRegistryStateAt(path)
	if err != nil {
		return nil, err
	}
	snapshot := &ProxySnapshot{}
	if state.enableSet {
		snapshot.Enabled = state.enable != 0
	}
	if state.serverSet && state.server != "" {
		snapshot.HasServer = true
		snapshot.Server = state.server
	}
	return snapshot, nil
}

func restore_proxy_snapshot(snapshot *ProxySnapshot) error {
	return restoreProxySnapshotAt(internetSettingsKeyPath, snapshot)
}

func restoreProxySnapshotAt(path string, snapshot *ProxySnapshot) error {
	if snapshot == nil {
		if err := setProxyEnableAt(path, false); err != nil {
			return fmt.Errorf("恢复系统代理开关失败: %w", err)
		}
		notify_proxy_changed()
		return nil
	}
	key, err := openInternetSettingsKeyForWriteAt(path)
	if err != nil {
		return err
	}
	defer key.Close()
	enable := uint32(0)
	if snapshot.Enabled {
		enable = 1
	}
	if err := key.SetDWordValue("ProxyEnable", enable); err != nil {
		return fmt.Errorf("恢复系统代理开关失败: %w", err)
	}
	if snapshot.HasServer {
		if err := key.SetStringValue("ProxyServer", snapshot.Server); err != nil {
			return fmt.Errorf("恢复系统代理地址失败: %w", err)
		}
	} else if err := key.DeleteValue("ProxyServer"); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("清理系统代理地址失败: %w", err)
	}
	notify_proxy_changed()
	return nil
}

func notify_proxy_changed() {
	wininet := windows.NewLazySystemDLL("wininet.dll")
	internetSetOption := wininet.NewProc("InternetSetOptionW")
	const (
		internetOptionRefresh         = 37
		internetOptionSettingsChanged = 39
	)
	_, _, _ = internetSetOption.Call(0, internetOptionSettingsChanged, 0, 0)
	_, _, _ = internetSetOption.Call(0, internetOptionRefresh, 0, 0)
}

func fetch_cur_proxy(args ProxySettings) (*ProxySettings, error) {
	return fetchCurrentProxyAt(internetSettingsKeyPath)
}

func fetchCurrentProxyAt(path string) (*ProxySettings, error) {
	state, err := readProxyRegistryStateAt(path)
	if err != nil {
		return nil, err
	}
	if !state.enableSet || state.enable == 0 || !state.serverSet || state.server == "" {
		return nil, nil
	}
	host, port, err := parse_proxy_server_value(state.server)
	if err != nil {
		return nil, err
	}
	if host == "" || port == "" {
		return nil, nil
	}
	return &ProxySettings{
		Hostname: host,
		Port:     port,
	}, nil
}

func get_network_interfaces() (*HardwarePort, error) {
	return nil, errors.New("not support")
}

// readProxyRegistryStateAt 读取指定注册表路径下的系统代理值。
// 键或值不存在表示“从未配置代理”，返回空状态而不是错误；中文 Windows 的
// reg.exe 错误文本编码不会再影响判断。
func readProxyRegistryStateAt(path string) (proxyRegistryState, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return proxyRegistryState{}, nil
	}
	if err != nil {
		return proxyRegistryState{}, fmt.Errorf("读取系统代理注册表失败: %w", err)
	}
	defer key.Close()

	var state proxyRegistryState
	value, _, err := key.GetIntegerValue("ProxyEnable")
	switch {
	case err == nil:
		state.enableSet = true
		state.enable = value
	case errors.Is(err, registry.ErrNotExist):
	default:
		return proxyRegistryState{}, fmt.Errorf("读取系统代理开关失败: %w", err)
	}
	server, _, err := key.GetStringValue("ProxyServer")
	switch {
	case err == nil:
		state.serverSet = true
		state.server = server
	case errors.Is(err, registry.ErrNotExist):
	default:
		return proxyRegistryState{}, fmt.Errorf("读取系统代理地址失败: %w", err)
	}
	return state, nil
}

func openInternetSettingsKeyForWrite() (registry.Key, error) {
	return openInternetSettingsKeyForWriteAt(internetSettingsKeyPath)
}

func openInternetSettingsKeyForWriteAt(path string) (registry.Key, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		key, _, err = registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	}
	if err != nil {
		return 0, fmt.Errorf("打开系统代理注册表失败: %w", err)
	}
	return key, nil
}

func set_proxy_enable(enabled bool) error {
	return setProxyEnableAt(internetSettingsKeyPath, enabled)
}

func setProxyEnableAt(path string, enabled bool) error {
	key, err := openInternetSettingsKeyForWriteAt(path)
	if err != nil {
		return err
	}
	defer key.Close()
	value := uint32(0)
	if enabled {
		value = 1
	}
	if err := key.SetDWordValue("ProxyEnable", value); err != nil {
		return fmt.Errorf("设置系统代理开关失败: %w", err)
	}
	return nil
}

func parse_proxy_server_value(value string) (string, string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", "", nil
	}
	parts := strings.Split(raw, ";")
	candidate := pick_proxy_candidate(parts, "http=")
	if candidate == "" {
		candidate = pick_proxy_candidate(parts, "https=")
	}
	if candidate == "" {
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if idx := strings.Index(part, "="); idx >= 0 {
				candidate = strings.TrimSpace(part[idx+1:])
				break
			}
		}
	}
	if candidate == "" {
		candidate = raw
	}
	return split_host_port(candidate)
}

func pick_proxy_candidate(parts []string, prefix string) string {
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), prefix) {
			return strings.TrimSpace(part[len(prefix):])
		}
	}
	return ""
}

func split_host_port(value string) (string, string, error) {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return "", "", nil
	}
	if strings.HasPrefix(candidate, "[") {
		host, port, err := net.SplitHostPort(candidate)
		if err != nil {
			return "", "", fmt.Errorf("解析系统代理地址失败: %v", err)
		}
		return host, port, nil
	}
	idx := strings.LastIndex(candidate, ":")
	if idx <= 0 || idx == len(candidate)-1 {
		return "", "", fmt.Errorf("解析系统代理地址失败: %s", candidate)
	}
	return candidate[:idx], candidate[idx+1:], nil
}
