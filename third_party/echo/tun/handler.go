package tun

import (
	"context"
	"net/netip"
	"sync"

	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"

	"github.com/ltaoo/echo/tun/routerhandler"
)

type tunHandler struct {
	ctx          context.Context
	searcher     *routerhandler.CachedSearcher
	logger       logger.Logger
	config       *TunConfig
	outbounds    map[string]outboundDialer
	directDialer *directDialer
	fakeDNS      *fakeDNS
	dnsMu        sync.RWMutex
	dnsCache     map[string][]netip.Addr
	tunAddr      netip.Addr
}

func (h *tunHandler) PrepareConnection(
	network string, source M.Socksaddr, destination M.Socksaddr,
) error {
	return nil
}
