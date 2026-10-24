//go:build !windows

package tun

import (
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/logger"
)

func addTunRoutes(tunName string, log logger.Logger) {}

func removeTunRoutes() {}

func dumpInterfaceDiagnostics(stage string, log logger.Logger) {}

func startRouteMonitor(networkMonitor tun.NetworkUpdateMonitor, tunName string, log logger.Logger) chan struct{} {
	stop := make(chan struct{})
	close(stop)
	return stop
}
