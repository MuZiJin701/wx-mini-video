package tun

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"golang.org/x/net/dns/dnsmessage"
)

func (h *tunHandler) parseDNSResponse(payload []byte) {
	var parser dnsmessage.Parser
	header, err := parser.Start(payload)
	if err != nil || !header.Response {
		return
	}
	if err := parser.SkipAllQuestions(); err != nil {
		return
	}
	for {
		ah, err := parser.AnswerHeader()
		if err != nil {
			break
		}
		domain := strings.TrimSuffix(ah.Name.String(), ".")
		switch ah.Type {
		case dnsmessage.TypeA:
			r, err := parser.AResource()
			if err != nil {
				return
			}
			h.addDNSCache(domain, netip.AddrFrom4(r.A))
		case dnsmessage.TypeAAAA:
			r, err := parser.AAAAResource()
			if err != nil {
				return
			}
			h.addDNSCache(domain, netip.AddrFrom16(r.AAAA))
		default:
			if err := parser.SkipAnswer(); err != nil {
				return
			}
		}
	}
}

func (h *tunHandler) addDNSCache(domain string, ip netip.Addr) {
	h.dnsMu.Lock()
	defer h.dnsMu.Unlock()
	ips := h.dnsCache[domain]
	for _, existing := range ips {
		if existing == ip {
			return
		}
	}
	h.dnsCache[domain] = append(ips, ip)
	h.logger.Debug(fmt.Sprintf("[dns] cached %s -> %v", domain, ip))
}

// tryFakeDNSResponse intercepts DNS queries for domains matching any rule's domain_suffix.
func (h *tunHandler) tryFakeDNSResponse(conn N.PacketConn, payload []byte, destination M.Socksaddr) bool {
	var parser dnsmessage.Parser
	header, err := parser.Start(payload)
	if err != nil || header.Response {
		return false
	}

	q, err := parser.Question()
	if err != nil {
		return false
	}

	domain := strings.TrimSuffix(q.Name.String(), ".")
	if q.Type != dnsmessage.TypeA || !h.matchesDomainSuffix(domain) {
		return false
	}

	fakeIP := h.fakeDNS.Lookup(domain)
	h.logger.Info(fmt.Sprintf("[fakedns] %s -> %v", domain, fakeIP))

	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{
		ID:                 header.ID,
		Response:           true,
		Authoritative:      true,
		RecursionDesired:   header.RecursionDesired,
		RecursionAvailable: true,
		RCode:              dnsmessage.RCodeSuccess,
	})
	builder.EnableCompression()
	builder.StartQuestions()
	builder.Question(q)
	builder.StartAnswers()
	ip4 := fakeIP.As4()
	builder.AResource(dnsmessage.ResourceHeader{
		Name:  q.Name,
		Type:  dnsmessage.TypeA,
		Class: dnsmessage.ClassINET,
		TTL:   60,
	}, dnsmessage.AResource{A: ip4})
	resp, err := builder.Finish()
	if err != nil {
		h.logger.Error(fmt.Sprintf("[fakedns] build response: %v", err))
		return false
	}

	respBuf := buf.NewPacket()
	respBuf.Write(resp)
	conn.WritePacket(respBuf, destination)
	return true
}
