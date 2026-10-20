package system

import "fmt"

func capture_proxy_snapshot() (*ProxySnapshot, error)      { return macCaptureProxy() }
func restore_proxy_snapshot(snapshot *ProxySnapshot) error { return macRestoreProxy(snapshot) }
func enable_proxy(args ProxySettings) error                { return macEnableProxy(args) }
func disable_proxy(args ProxySettings) error {
	if args.Device == "" {
		return fmt.Errorf("未指定网络服务，未更改系统代理")
	}
	return RunMacAdmin([][]string{{"/usr/sbin/networksetup", "-setwebproxystate", args.Device, "off"}, {"/usr/sbin/networksetup", "-setsecurewebproxystate", args.Device, "off"}})
}
func fetch_cur_proxy(args ProxySettings) (*ProxySettings, error) {
	snapshot, err := macCaptureProxy()
	if err != nil {
		return nil, err
	}
	if !snapshot.Mac.HTTP.Enabled {
		return nil, nil
	}
	return &ProxySettings{Device: snapshot.Mac.Service, Hostname: snapshot.Mac.HTTP.Server, Port: snapshot.Mac.HTTP.Port}, nil
}
func get_network_interfaces() (*HardwarePort, error) {
	iface, err := macActiveInterface()
	return &HardwarePort{Interface: iface}, err
}
