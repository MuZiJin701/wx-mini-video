package system

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestMacProxySnapshotRejectsOtherProxiesAndClearsOwnLeftovers(t *testing.T) {
	s := &MacProxySnapshot{Service: "Wi-Fi", HTTP: MacWebProxy{Enabled: true, Server: "127.0.0.1", Port: "2023"}, HTTPS: MacWebProxy{Enabled: true, Server: "localhost", Port: "2023"}}
	if err := s.Prepare("127.0.0.1", "2023"); err != nil || s.HTTP.Enabled || s.HTTPS.Enabled {
		t.Fatalf("leftover = %+v, %v", s, err)
	}
	s.HTTP = MacWebProxy{Enabled: true, Server: "proxy.example", Port: "8080"}
	if err := s.Prepare("127.0.0.1", "2023"); err == nil || !s.HTTP.Enabled {
		t.Fatal("accepted a conflicting proxy")
	}
	s.HTTP.Server, s.HTTP.Port = "127.0.0.1", "20230"
	if err := s.Prepare("127.0.0.1", "2023"); err == nil {
		t.Fatal("matched a port prefix")
	}
}

func TestMacCaptureSelectsActiveServiceAndRejectsPAC(t *testing.T) {
	oldRun := macRun
	t.Cleanup(func() { macRun = oldRun })
	pac := false
	macRun = func(name string, args ...string) ([]byte, error) {
		if name == "/sbin/route" {
			return []byte("route to: default\n interface: en5\n"), nil
		}
		switch args[0] {
		case "-listnetworkserviceorder":
			return []byte("(1) Wi-Fi\n(Hardware Port: Wi-Fi, Device: en0)\n(2) USB Ethernet's adapter\n(Hardware Port: USB LAN, Device: en5)\n"), nil
		case "-getautoproxyurl":
			if pac {
				return []byte("URL: https://proxy.example/pac\nEnabled: Yes\n"), nil
			}
			return []byte("Enabled: No\n"), nil
		case "-getproxyautodiscovery":
			return []byte("Auto Proxy Discovery: Off\n"), nil
		case "-getsocksfirewallproxy":
			return []byte("Enabled: No\n"), nil
		case "-getwebproxy", "-getsecurewebproxy":
			if args[1] != "USB Ethernet's adapter" {
				t.Fatal("selected inactive service")
			}
			return []byte("Enabled: No\nServer: \nPort: 0\nAuthenticated Proxy Enabled: 0\n"), nil
		}
		t.Fatalf("unexpected command: %s %v", name, args)
		return nil, nil
	}
	snapshot, err := macCaptureProxy()
	if err != nil || snapshot.Mac.Service != "USB Ethernet's adapter" || snapshot.Mac.Interface != "en5" || snapshot.Mac.HTTP.Port != "0" {
		t.Fatalf("snapshot = %+v, %v", snapshot, err)
	}
	pac = true
	if _, err := macCaptureProxy(); err == nil {
		t.Fatal("accepted enabled PAC")
	}
}

func TestMacRestoreAttemptsBothProtocolsAndPreservesDisabledEndpoints(t *testing.T) {
	oldAdmin := macAdmin
	t.Cleanup(func() { macAdmin = oldAdmin })
	var calls [][][]string
	failure := errors.New("first restore denied")
	macAdmin = func(commands [][]string) error {
		calls = append(calls, commands)
		if len(calls) == 1 {
			return failure
		}
		return nil
	}
	snapshot := &ProxySnapshot{Mac: &MacProxySnapshot{Service: "User's LAN", HTTP: MacWebProxy{Port: "0"}, HTTPS: MacWebProxy{Enabled: true, Server: "old.example", Port: "8443"}}}
	if err := macRestoreProxy(snapshot); !errors.Is(err, failure) {
		t.Fatalf("restore error = %v", err)
	}
	if len(calls) != 2 {
		t.Fatal("did not attempt both protocols after failure")
	}
	if !reflect.DeepEqual(calls[0][0], []string{"/usr/sbin/networksetup", "-setwebproxystate", "User's LAN", "off"}) {
		t.Fatal("must disable before restoring a possibly empty endpoint")
	}
	if calls[0][1][3] != "" || calls[0][1][4] != "0" || calls[1][2][3] != "on" {
		t.Fatalf("lost original state: %v", calls)
	}
	before := len(calls)
	if macRestoreProxy(nil) == nil || macEnableProxy(ProxySettings{}) == nil || len(calls) != before {
		t.Fatal("changed proxy without a captured service")
	}
}

func TestMacNetworkSwitchAndVPNAreReported(t *testing.T) {
	oldRun := macRun
	t.Cleanup(func() { macRun = oldRun })
	iface := "en0"
	macRun = func(string, ...string) ([]byte, error) { return []byte("interface: " + iface + "\n"), nil }
	s := &MacProxySnapshot{Interface: "en0"}
	if err := s.CheckNetwork(); err != nil {
		t.Fatal(err)
	}
	iface = "en5"
	if err := s.CheckNetwork(); err == nil {
		t.Fatal("missed network switch")
	}
	iface = "utun4"
	if _, err := macActiveInterface(); err == nil {
		t.Fatal("accepted VPN default route")
	}
}

func TestMacProxyParsingAndShellQuoting(t *testing.T) {
	proxy, err := parseMacWebProxy("Enabled: Yes\nServer: ::1\nPort: 2023\nAuthenticated Proxy Enabled: 1\n")
	if err != nil || !proxy.Enabled || !proxy.Authenticated || proxy.Server != "::1" {
		t.Fatalf("proxy = %+v, %v", proxy, err)
	}
	if _, err := parseMacWebProxy("unexpected localized or partial output"); err == nil {
		t.Fatal("accepted unknown proxy state")
	}
	value := "User's LAN; $(touch /tmp/should-not-run)"
	quoted := ShellQuote(value)
	if quoted != "'User'\"'\"'s LAN; $(touch /tmp/should-not-run)'" || !strings.HasPrefix(quoted, "'") {
		t.Fatal("unsafe shell argument")
	}
	script, err := macAdminScript([][]string{{"/usr/sbin/networksetup", "-setwebproxy", value, "127.0.0.1", "2023", "off"}})
	if err != nil || !strings.Contains(script, quoted) || !strings.Contains(script, "** Error:") {
		t.Fatal("admin command lost quoting or error detection")
	}
	if _, err := macAdminScript([][]string{{"sh", "-c", "anything"}}); err == nil {
		t.Fatal("accepted an arbitrary privileged executable")
	}
}
