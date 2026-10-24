package tun

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"github.com/sagernet/sing/common/control"
	M "github.com/sagernet/sing/common/metadata"
)

type directDialer struct {
	ifaceFinder control.InterfaceFinder
	ifaceName   string
	ifaceIndex  int
}

func (d *directDialer) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	var dialer net.Dialer
	if !destination.Addr.IsLoopback() {
		dialer.Control = control.BindToInterfaceFunc(d.ifaceFinder, func(network, address string) (string, int, error) {
			return d.ifaceName, d.ifaceIndex, nil
		})
	}
	return dialer.DialContext(ctx, network, destination.String())
}

func (d *directDialer) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	var lc net.ListenConfig
	lc.Control = control.BindToInterfaceFunc(d.ifaceFinder, func(network, address string) (string, int, error) {
		return d.ifaceName, d.ifaceIndex, nil
	})
	return lc.ListenPacket(ctx, "udp", "")
}

// ResolveIP resolves a domain to an IPv4 address using AliDNS (223.5.5.5)
// reached through the bound physical interface, bypassing TUN and fakeDNS.
func (d *directDialer) ResolveIP(ctx context.Context, domain string) (netip.Addr, error) {
	dd := d
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			var dialer net.Dialer
			dialer.Control = control.BindToInterfaceFunc(dd.ifaceFinder, func(network, address string) (string, int, error) {
				return dd.ifaceName, dd.ifaceIndex, nil
			})
			return dialer.DialContext(ctx, network, "223.5.5.5:53")
		},
	}
	ips, err := resolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return netip.Addr{}, err
	}
	for _, ip := range ips {
		if ip4 := ip.IP.To4(); ip4 != nil {
			return netip.AddrFrom4([4]byte(ip4)), nil
		}
	}
	return netip.Addr{}, fmt.Errorf("no IPv4 address for %s", domain)
}
