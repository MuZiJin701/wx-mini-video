package tun

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"path/filepath"
	"time"

	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/ltaoo/echo/tun/routerhandler"
)

func (h *tunHandler) NewConnectionEx(
	ctx context.Context, conn net.Conn,
	source M.Socksaddr, destination M.Socksaddr,
	onClose N.CloseHandlerFunc,
) {
	src := netip.AddrPortFrom(source.Addr, source.Port)
	dst := netip.AddrPortFrom(destination.Addr, destination.Port)

	// Drop connections to TUN-local addresses (SSDP, loopback, service discovery)
	if destination.Addr == h.tunAddr || destination.Addr == h.tunAddr.Next() {
		h.logger.Debug(fmt.Sprintf("[tcp] drop tun-local %v -> %v", src, dst))
		conn.Close()
		return
	}

	// 1. Look up which process owns this connection
	owner, err := routerhandler.FindProcessInfo(h.searcher, h.ctx, "tcp", src, dst)
	if err != nil {
		h.logger.Debug(fmt.Sprintf("[tcp] process lookup %v->%v: %v", src, dst, err))
	}
	processPath := "unknown"
	if owner != nil && owner.ProcessPath != "" {
		processPath = owner.ProcessPath
	}

	// 2. Sniff TLS ClientHello for domain
	var sniffedDomain string
	var peeked []byte
	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	peek := make([]byte, 2048)
	n, _ := conn.Read(peek)
	conn.SetReadDeadline(time.Time{})
	if n > 0 {
		peeked = peek[:n]
		sniffedDomain = extractSNI(peeked)
	}

	// 2.5. Check fake DNS reverse lookup
	fakeDomain := h.fakeDNS.Reverse(destination.Addr)
	if fakeDomain != "" && sniffedDomain == "" {
		sniffedDomain = fakeDomain
	}

	// 3. Determine routing via config rules
	outboundTag := h.matchRoute(owner, destination, sniffedDomain)

	if sniffedDomain != "" {
		h.logger.Info(fmt.Sprintf("[tcp] %v -> %v [%s] | %s | %s", src, dst, sniffedDomain, processPath, outboundTag))
	} else {
		h.logger.Info(fmt.Sprintf("[tcp] %v -> %v | %s | %s", src, dst, processPath, outboundTag))
	}

	// 4. Dial outbound
	ob, ok := h.outbounds[outboundTag]
	if !ok {
		h.logger.Error(fmt.Sprintf("[tcp] unknown outbound tag: %s, falling back to %s", outboundTag, h.config.Route.Final))
		ob = h.outbounds[h.config.Route.Final]
		if ob == nil {
			h.logger.Error("[tcp] no fallback outbound available")
			conn.Close()
			return
		}
	}

	dialDest := destination
	if sniffedDomain != "" {
		dialDest = M.ParseSocksaddr(fmt.Sprintf("%s:%d", sniffedDomain, destination.Port))
		if outboundTag == "direct" {
			if ip, err := h.directDialer.ResolveIP(context.Background(), sniffedDomain); err == nil {
				dialDest = M.ParseSocksaddr(fmt.Sprintf("%s:%d", ip.String(), destination.Port))
			}
		}
	}

	outbound, err := ob.DialContext(context.Background(), "tcp", dialDest)
	if err != nil {
		h.logger.Error(fmt.Sprintf("[tcp] dial %v via %s: %v", dialDest, outboundTag, err))
		conn.Close()
		return
	}

	// 5. Replay peeked bytes + bidirectional copy
	var reader io.Reader
	if len(peeked) > 0 {
		reader = io.MultiReader(bytes.NewReader(peeked), conn)
	} else {
		reader = conn
	}

	go func() {
		io.Copy(outbound, reader)
		outbound.Close()
	}()
	io.Copy(conn, outbound)
	conn.Close()
}

func (h *tunHandler) processBaseName(owner *routerhandler.ConnectionOwner) string {
	if owner != nil && owner.ProcessPath != "" {
		return filepath.Base(owner.ProcessPath)
	}
	return ""
}
