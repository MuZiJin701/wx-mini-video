package tun

import (
	"net/netip"
	"path/filepath"
	"strings"

	M "github.com/sagernet/sing/common/metadata"

	"github.com/ltaoo/echo/tun/routerhandler"
)

func (h *tunHandler) collectDomainSuffixes() []string {
	var suffixes []string
	for _, rule := range h.config.Route.Rules {
		suffixes = append(suffixes, rule.DomainSuffix...)
	}
	return suffixes
}

func (h *tunHandler) matchesDomainSuffix(domain string) bool {
	for _, suffix := range h.collectDomainSuffixes() {
		if domain == suffix || strings.HasSuffix(domain, "."+suffix) {
			return true
		}
	}
	return false
}

// matchRoute evaluates routing rules and returns the outbound tag.
func (h *tunHandler) matchRoute(owner *routerhandler.ConnectionOwner, dest M.Socksaddr, sniffedDomain string) string {
	processName := ""
	if owner != nil && owner.ProcessPath != "" {
		processName = filepath.Base(owner.ProcessPath)
	}

	for _, rule := range h.config.Route.Rules {
		matched := true

		if len(rule.ProcessName) > 0 {
			nameMatch := false
			for _, pn := range rule.ProcessName {
				if pn == processName {
					nameMatch = true
					break
				}
			}
			if !nameMatch {
				matched = false
			}
		}

		if matched && len(rule.DomainSuffix) > 0 {
			domain := h.resolveDomain(dest, sniffedDomain)
			suffixMatch := false
			for _, suffix := range rule.DomainSuffix {
				if domain == suffix || strings.HasSuffix(domain, "."+suffix) {
					suffixMatch = true
					break
				}
			}
			if !suffixMatch {
				matched = false
			}
		}

		if matched && len(rule.Domain) > 0 {
			domain := h.resolveDomain(dest, sniffedDomain)
			domainMatch := false
			for _, d := range rule.Domain {
				if domain == d {
					domainMatch = true
					break
				}
			}
			if !domainMatch {
				matched = false
			}
		}

		if matched && len(rule.IPCidr) > 0 {
			cidrMatch := false
			for _, cidr := range rule.IPCidr {
				prefix, err := netip.ParsePrefix(cidr)
				if err != nil {
					continue
				}
				if prefix.Contains(dest.Addr) {
					cidrMatch = true
					break
				}
			}
			if !cidrMatch {
				matched = false
			}
		}

		if matched && len(rule.Port) > 0 {
			portMatch := false
			for _, p := range rule.Port {
				if p == dest.Port {
					portMatch = true
					break
				}
			}
			if !portMatch {
				matched = false
			}
		}

		if rule.Invert {
			matched = !matched
		}

		if matched {
			return rule.Outbound
		}
	}

	// Check fake IP — if destination is a fake IP, find the outbound for its domain
	if h.fakeDNS.IsFakeIP(dest.Addr) {
		domain := h.fakeDNS.Reverse(dest.Addr)
		if domain != "" {
			for _, rule := range h.config.Route.Rules {
				if len(rule.DomainSuffix) > 0 {
					for _, suffix := range rule.DomainSuffix {
						if domain == suffix || strings.HasSuffix(domain, "."+suffix) {
							return rule.Outbound
						}
					}
				}
				if len(rule.Domain) > 0 {
					for _, d := range rule.Domain {
						if domain == d {
							return rule.Outbound
						}
					}
				}
			}
		}
	}

	return h.config.Route.Final
}

// resolveDomain returns the best known domain for a connection.
func (h *tunHandler) resolveDomain(dest M.Socksaddr, sniffedDomain string) string {
	if sniffedDomain != "" {
		return sniffedDomain
	}
	if domain := h.fakeDNS.Reverse(dest.Addr); domain != "" {
		return domain
	}
	h.dnsMu.RLock()
	defer h.dnsMu.RUnlock()
	for domain, ips := range h.dnsCache {
		for _, ip := range ips {
			if ip == dest.Addr {
				return domain
			}
		}
	}
	return ""
}
