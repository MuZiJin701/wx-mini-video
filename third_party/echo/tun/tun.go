package tun

import (
	"context"
	"fmt"
	"net/netip"
	"runtime"
	"strings"
	"time"

	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	singHTTP "github.com/sagernet/sing/protocol/http"
	singSocks "github.com/sagernet/sing/protocol/socks"

	"github.com/ltaoo/echo/tun/routerhandler"
)

// Server manages the TUN device lifecycle and traffic routing.
type Server struct {
	cfg            *TunConfig
	log            logger.Logger
	searcher       *routerhandler.CachedSearcher
	ifaceFinder    control.InterfaceFinder
	networkMonitor tun.NetworkUpdateMonitor
	ifaceMonitor   tun.DefaultInterfaceMonitor
	tunIf          tun.Tun
	tunStack       tun.Stack
	routeStop      chan struct{}
}

// New creates a new TUN Server from config. Does NOT start the TUN device yet.
func New(cfg *TunConfig) (*Server, error) {
	log := stdLogger{}

	// 1. Create process searcher
	searcher, err := routerhandler.NewSearcher(routerhandler.Config{
		Logger: func(format string, args ...interface{}) {
			fmt.Fprintf(logWriter, "[process] "+format+"\n", args...)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("NewSearcher: %w", err)
	}
	cachedSearcher := routerhandler.NewCachedSearcher(searcher, 256, 200*time.Millisecond)

	// 2. Create interface finder + network monitor
	ifaceFinder := control.NewDefaultInterfaceFinder()
	if err := ifaceFinder.Update(); err != nil {
		searcher.Close()
		return nil, fmt.Errorf("interface finder update: %w", err)
	}

	networkMonitor, err := tun.NewNetworkUpdateMonitor(log)
	if err != nil {
		searcher.Close()
		return nil, fmt.Errorf("network monitor: %w", err)
	}
	networkMonitor.Start()

	ifaceMonitor, err := tun.NewDefaultInterfaceMonitor(networkMonitor, log, tun.DefaultInterfaceMonitorOptions{
		InterfaceFinder: ifaceFinder,
	})
	if err != nil {
		networkMonitor.Close()
		searcher.Close()
		return nil, fmt.Errorf("interface monitor: %w", err)
	}
	if err := ifaceMonitor.Start(); err != nil {
		networkMonitor.Close()
		searcher.Close()
		return nil, fmt.Errorf("start interface monitor: %w", err)
	}

	return &Server{
		cfg:            cfg,
		log:            log,
		searcher:       cachedSearcher,
		ifaceFinder:    ifaceFinder,
		networkMonitor: networkMonitor,
		ifaceMonitor:   ifaceMonitor,
	}, nil
}

// Start creates the TUN device, stack, and begins processing traffic.
func (s *Server) Start() error {
	// 1. Detect default physical interface
	defaultIface, err := s.resolveDefaultInterface()
	if err != nil {
		return err
	}

	// 2. Build TUN options
	tunOptions := tun.Options{
		Name:                                  s.cfg.Inbound.TunName,
		Inet4Address:                          []netip.Prefix{netip.MustParsePrefix(s.cfg.Inbound.Inet4Address)},
		MTU:                                   s.cfg.Inbound.MTU,
		AutoRoute:                             s.cfg.Inbound.AutoRoute,
		StrictRoute:                           s.cfg.Inbound.StrictRoute,
		IPRoute2TableIndex:                    tun.DefaultIPRoute2TableIndex,
		IPRoute2RuleIndex:                     tun.DefaultIPRoute2RuleIndex,
		IPRoute2AutoRedirectFallbackRuleIndex: tun.DefaultIPRoute2AutoRedirectFallbackRuleIndex,
		InterfaceMonitor:                      s.ifaceMonitor,
		InterfaceFinder:                       s.ifaceFinder,
		Logger:                                s.log,
	}

	tunName := tunOptions.Name
	if tunName == "" {
		tunName = tun.CalculateInterfaceName("")
		tunOptions.Name = tunName
	}

	// 3. Create TUN device
	tunIf, err := tun.New(tunOptions)
	if err != nil {
		return fmt.Errorf("tun.New: %w", err)
	}
	s.tunIf = tunIf

	// 4. Create direct dialer bound to physical interface
	dd := &directDialer{
		ifaceFinder: s.ifaceFinder,
		ifaceName:   defaultIface.Name,
		ifaceIndex:  defaultIface.Index,
	}

	// 5. Build outbounds map
	outboundsMap := make(map[string]outboundDialer)
	for _, obCfg := range s.cfg.Outbounds {
		switch obCfg.Type {
		case "direct":
			outboundsMap[obCfg.Tag] = &directOutbound{
				tag:    obCfg.Tag,
				dialer: dd,
			}
		case "http":
			serverAddr := fmt.Sprintf("%s:%d", obCfg.Server, obCfg.Port)
			client := singHTTP.NewClient(singHTTP.Options{
				Dialer: dd,
				Server: M.ParseSocksaddr(serverAddr),
			})
			outboundsMap[obCfg.Tag] = &httpOutbound{
				tag:    obCfg.Tag,
				client: client,
			}
		case "socks5":
			serverAddr := M.ParseSocksaddr(fmt.Sprintf("%s:%d", obCfg.Server, obCfg.Port))
			outboundsMap[obCfg.Tag] = &socks5Outbound{
				tag:    obCfg.Tag,
				client: singSocks.NewClient(dd, serverAddr, singSocks.Version5, obCfg.Username, obCfg.Password),
			}
		default:
			s.log.Warn("unknown outbound type: ", obCfg.Type, " for tag: ", obCfg.Tag)
		}
	}

	// 6. Create routing handler
	h := &tunHandler{
		ctx:          context.Background(),
		searcher:     s.searcher,
		logger:       s.log,
		config:       s.cfg,
		outbounds:    outboundsMap,
		directDialer: dd,
		fakeDNS:      newFakeDNS(),
		dnsCache:     make(map[string][]netip.Addr),
		tunAddr:      netip.MustParsePrefix(s.cfg.Inbound.Inet4Address).Addr(),
	}

	// 7. Create network stack (gVisor on Windows, system on others)
	stackType := ""
	if runtime.GOOS == "windows" {
		stackType = "gvisor"
	}
	tunStack, err := tun.NewStack(stackType, tun.StackOptions{
		Context:         context.Background(),
		Tun:             tunIf,
		TunOptions:      tunOptions,
		UDPTimeout:      5 * time.Minute,
		Handler:         h,
		Logger:          s.log,
		InterfaceFinder: s.ifaceFinder,
	})
	if err != nil {
		tunIf.Close()
		return fmt.Errorf("tun.NewStack: %w", err)
	}
	s.tunStack = tunStack

	// 8. Start stack first, then TUN interface
	if err := tunStack.Start(); err != nil {
		tunIf.Close()
		return fmt.Errorf("stack.Start: %w", err)
	}
	if err := tunIf.Start(); err != nil {
		tunStack.Close()
		dumpInterfaceDiagnostics("tun.Start failed", s.log)
		return fmt.Errorf("tun.Start: %w", err)
	}
	if strings.TrimSpace(s.cfg.Route.DefaultInterface) != "" {
		s.log.Info("tun.Start bind interface: ", defaultIface.Name, " (index ", defaultIface.Index, ")")
	} else if currentIface := s.ifaceMonitor.DefaultInterface(); currentIface != nil {
		s.log.Info("tun.Start default interface: ", currentIface.Name, " (index ", currentIface.Index, ")")
	} else {
		s.log.Warn("tun.Start completed but default interface monitor is empty")
	}
	dumpInterfaceDiagnostics("after tun.Start", s.log)

	// 9. Windows route management
	if runtime.GOOS == "windows" {
		addTunRoutes(tunName, s.log)
		s.routeStop = startRouteMonitor(s.networkMonitor, tunName, s.log)
	}

	s.log.Info("============================================")
	s.log.Info("TUN forwarder running")
	s.log.Info("  interface:  ", tunName)
	s.log.Info("  address:    ", s.cfg.Inbound.Inet4Address)
	s.log.Info("  bind:       ", defaultIface.Name)
	s.log.Info("  outbounds:  ", fmt.Sprintf("%d configured", len(outboundsMap)))
	s.log.Info("  rules:      ", fmt.Sprintf("%d rules, final=%s", len(s.cfg.Route.Rules), s.cfg.Route.Final))
	s.log.Info("  fake_dns:   ", fmt.Sprintf("%v", s.cfg.DNS.FakeDNS))
	s.log.Info("============================================")

	return nil
}

func (s *Server) resolveDefaultInterface() (*control.Interface, error) {
	configuredInterface := strings.TrimSpace(s.cfg.Route.DefaultInterface)
	if configuredInterface != "" {
		if err := s.ifaceFinder.Update(); err != nil {
			dumpInterfaceDiagnostics("configured default_interface update failed", s.log)
			return nil, fmt.Errorf("update interface finder for default_interface %q: %w", configuredInterface, err)
		}
		defaultIface, err := s.ifaceFinder.ByName(configuredInterface)
		if err != nil {
			dumpInterfaceDiagnostics("configured default_interface not found", s.log)
			return nil, fmt.Errorf("default_interface %q not found: %w", configuredInterface, err)
		}
		s.log.Info("using configured default interface: ", defaultIface.Name, " (index ", defaultIface.Index, ")")
		return defaultIface, nil
	}

	defaultIface := s.ifaceMonitor.DefaultInterface()
	if defaultIface == nil {
		dumpInterfaceDiagnostics("default interface missing before tun.Start", s.log)
		return nil, fmt.Errorf("no default network interface detected - check your network connection")
	}
	s.log.Info("detected default interface: ", defaultIface.Name, " (index ", defaultIface.Index, ")")
	return defaultIface, nil
}

// Close stops the TUN device and cleans up resources.
func (s *Server) Close() error {
	if s.routeStop != nil {
		close(s.routeStop)
		removeTunRoutes()
	}
	if s.tunStack != nil {
		s.tunStack.Close()
	}
	if s.tunIf != nil {
		s.tunIf.Close()
	}
	if s.ifaceMonitor != nil {
		s.ifaceMonitor.Close()
	}
	if s.networkMonitor != nil {
		s.networkMonitor.Close()
	}
	if s.searcher != nil {
		s.searcher.Close()
	}
	return nil
}
