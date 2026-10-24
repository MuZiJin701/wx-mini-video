package tun

import (
	"net/netip"
	"sync"
)

type fakeDNS struct {
	mu         sync.RWMutex
	domainToIP map[string]netip.Addr
	ipToDomain map[netip.Addr]string
	nextIP     netip.Addr
}

func newFakeDNS() *fakeDNS {
	return &fakeDNS{
		domainToIP: make(map[string]netip.Addr),
		ipToDomain: make(map[netip.Addr]string),
		nextIP:     netip.MustParseAddr("198.18.0.1"),
	}
}

func (f *fakeDNS) Lookup(domain string) netip.Addr {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ip, ok := f.domainToIP[domain]; ok {
		return ip
	}
	ip := f.nextIP
	f.domainToIP[domain] = ip
	f.ipToDomain[ip] = domain
	f.nextIP = f.nextIP.Next()
	return ip
}

func (f *fakeDNS) Reverse(ip netip.Addr) string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.ipToDomain[ip]
}

func (f *fakeDNS) IsFakeIP(ip netip.Addr) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	_, ok := f.ipToDomain[ip]
	return ok
}
