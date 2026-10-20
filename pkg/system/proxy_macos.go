package system

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

type MacWebProxy struct {
	Enabled       bool
	Server        string
	Port          string
	Authenticated bool
}

type MacProxySnapshot struct {
	Service   string
	Interface string
	HTTP      MacWebProxy
	HTTPS     MacWebProxy
}

var macRun = RunMacCommand
var macAdmin = RunMacAdmin

func parseMacWebProxy(output string) (MacWebProxy, error) {
	values := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		if key, value, ok := strings.Cut(line, ":"); ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	if values["Enabled"] != "Yes" && values["Enabled"] != "No" {
		return MacWebProxy{}, fmt.Errorf("无法解析 macOS 代理状态")
	}
	if values["Port"] == "" {
		return MacWebProxy{}, fmt.Errorf("macOS 代理端口缺失")
	}
	if value := values["Authenticated Proxy Enabled"]; value != "0" && value != "1" && value != "Yes" && value != "No" {
		return MacWebProxy{}, fmt.Errorf("macOS 代理认证状态缺失或无法识别")
	}
	port, err := strconv.Atoi(values["Port"])
	if err != nil || port < 0 || port > 65535 || (values["Enabled"] == "Yes" && (port == 0 || values["Server"] == "")) {
		return MacWebProxy{}, fmt.Errorf("macOS 代理地址或端口无效")
	}
	return MacWebProxy{Enabled: values["Enabled"] == "Yes", Server: values["Server"], Port: values["Port"], Authenticated: values["Authenticated Proxy Enabled"] == "1" || values["Authenticated Proxy Enabled"] == "Yes"}, nil
}

func macActiveInterface() (string, error) {
	output, err := macRun("/sbin/route", "-n", "get", "default")
	if err != nil {
		output, err = macRun("/sbin/route", "-n", "get", "-inet6", "default")
	}
	if err != nil {
		return "", fmt.Errorf("未找到活动网络，请连接 Wi-Fi 或有线网络: %w", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), ":"); ok && key == "interface" {
			iface := strings.TrimSpace(value)
			if strings.HasPrefix(iface, "utun") || strings.HasPrefix(iface, "ppp") || strings.HasPrefix(iface, "tun") {
				return "", fmt.Errorf("检测到 VPN 网络，请先断开 VPN 后重试")
			}
			if iface != "" {
				return iface, nil
			}
		}
	}
	return "", fmt.Errorf("无法确定活动网络接口")
}

func macNetworkService(iface, output string) (string, error) {
	service := ""
	order := regexp.MustCompile(`^\([0-9]+\) (.+)$`)
	device := regexp.MustCompile(`Device:\s*([^,)]+)`)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if match := order.FindStringSubmatch(line); match != nil {
			service = match[1]
		}
		if match := device.FindStringSubmatch(line); match != nil && strings.TrimSpace(match[1]) == iface && service != "" {
			return service, nil
		}
	}
	return "", fmt.Errorf("未找到接口 %s 对应的网络服务，请停用 VPN 或其他虚拟网络后重试", iface)
}

func macCaptureProxy() (*ProxySnapshot, error) {
	iface, err := macActiveInterface()
	if err != nil {
		return nil, err
	}
	output, err := macRun("/usr/sbin/networksetup", "-listnetworkserviceorder")
	if err != nil {
		return nil, err
	}
	service, err := macNetworkService(iface, string(output))
	if err != nil {
		return nil, err
	}
	for _, flag := range []string{"-getautoproxyurl", "-getproxyautodiscovery", "-getsocksfirewallproxy"} {
		output, err := macRun("/usr/sbin/networksetup", flag, service)
		if err != nil {
			return nil, err
		}
		if !strings.Contains(string(output), "Enabled: No") && !strings.Contains(string(output), "Enabled: Yes") && !strings.Contains(string(output), ": On") && !strings.Contains(string(output), ": Off") {
			return nil, fmt.Errorf("无法识别 macOS 自动代理或 SOCKS 设置，未更改系统代理")
		}
		if strings.Contains(string(output), "Enabled: Yes") || strings.Contains(string(output), ": On") {
			return nil, fmt.Errorf("网络服务 %s 已启用 PAC、自动代理发现或 SOCKS 代理，请先停用冲突设置后重试", service)
		}
	}
	state := &MacProxySnapshot{Service: service, Interface: iface}
	for _, item := range []struct {
		flag string
		dst  *MacWebProxy
	}{{"-getwebproxy", &state.HTTP}, {"-getsecurewebproxy", &state.HTTPS}} {
		output, err := macRun("/usr/sbin/networksetup", item.flag, service)
		if err != nil {
			return nil, err
		}
		*item.dst, err = parseMacWebProxy(string(output))
		if err != nil {
			return nil, err
		}
		if item.dst.Authenticated {
			return nil, fmt.Errorf("网络服务 %s 配置了需认证的代理，请先停用并移除该代理配置，避免丢失其凭据", service)
		}
	}
	return &ProxySnapshot{Mac: state}, nil
}

func (s *MacProxySnapshot) Prepare(host, port string) error {
	for _, state := range []*MacWebProxy{&s.HTTP, &s.HTTPS} {
		if !state.Enabled {
			continue
		}
		if state.Port != port || !(strings.EqualFold(state.Server, host) || state.Server == "localhost" || state.Server == "127.0.0.1" || state.Server == "::1") {
			return fmt.Errorf("网络服务 %s 已启用其他 HTTP/HTTPS 代理，请先停用后重试", s.Service)
		}
		// A matching loopback proxy is a leftover from an interrupted run.
		state.Enabled = false
	}
	return nil
}

// shortcut: only interface changes pause sniffing; track service state if hotspot changes must also pause it.
func (s *MacProxySnapshot) CheckNetwork() error {
	iface, err := macActiveInterface()
	if err != nil {
		return err
	}
	if iface != s.Interface {
		return fmt.Errorf("网络已切换，请按 r 重新启动代理")
	}
	return nil
}

func macEnableProxy(args ProxySettings) error {
	if args.Device == "" {
		return fmt.Errorf("缺少已捕获快照的网络服务，未更改系统代理")
	}
	if _, _, err := net.SplitHostPort(net.JoinHostPort(args.Hostname, args.Port)); err != nil {
		return err
	}
	port, err := strconv.Atoi(args.Port)
	if err != nil || port < 1 || port > 65535 || args.Hostname == "" {
		return fmt.Errorf("无效的本机代理地址或端口")
	}
	return macAdmin([][]string{
		{"/usr/sbin/networksetup", "-setwebproxy", args.Device, args.Hostname, args.Port, "off"},
		{"/usr/sbin/networksetup", "-setsecurewebproxy", args.Device, args.Hostname, args.Port, "off"},
		{"/usr/sbin/networksetup", "-setwebproxystate", args.Device, "on"},
		{"/usr/sbin/networksetup", "-setsecurewebproxystate", args.Device, "on"},
	})
}

func macRestoreProxy(snapshot *ProxySnapshot) error {
	if snapshot == nil || snapshot.Mac == nil {
		return fmt.Errorf("缺少 macOS 代理快照，未更改系统代理")
	}
	s := snapshot.Mac
	var result error
	for _, item := range []struct {
		set, toggle string
		state       MacWebProxy
	}{{"-setwebproxy", "-setwebproxystate", s.HTTP}, {"-setsecurewebproxy", "-setsecurewebproxystate", s.HTTPS}} {
		enabled := "off"
		if item.state.Enabled {
			enabled = "on"
		}
		err := macAdmin([][]string{
			{"/usr/sbin/networksetup", item.toggle, s.Service, "off"},
			{"/usr/sbin/networksetup", item.set, s.Service, item.state.Server, item.state.Port, "off"},
			{"/usr/sbin/networksetup", item.toggle, s.Service, enabled},
		})
		result = errors.Join(result, err)
	}
	return result
}
