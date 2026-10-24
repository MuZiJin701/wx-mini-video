//go:build windows

package windows

import (
	"encoding/binary"
	"fmt"
	"net"
)

// IPv4Header provides read/write access to IPv4 header fields in a raw packet buffer.
type IPv4Header struct {
	raw []byte
}

// HeaderLen returns the IPv4 header length in bytes.
func (h *IPv4Header) HeaderLen() int {
	return int(h.raw[0]&0x0F) * 4
}

// TotalLen returns the total packet length from the IPv4 header.
func (h *IPv4Header) TotalLen() uint16 {
	return binary.BigEndian.Uint16(h.raw[2:4])
}

// Protocol returns the IP protocol number (6 = TCP, 17 = UDP).
func (h *IPv4Header) Protocol() uint8 {
	return h.raw[9]
}

// SrcAddr returns the source IP address as a 4-byte network-order uint32.
func (h *IPv4Header) SrcAddr() uint32 {
	return binary.BigEndian.Uint32(h.raw[12:16])
}

// SetSrcAddr sets the source IP address (network byte order).
func (h *IPv4Header) SetSrcAddr(ip uint32) {
	binary.BigEndian.PutUint32(h.raw[12:16], ip)
}

// DstAddr returns the destination IP address as a 4-byte network-order uint32.
func (h *IPv4Header) DstAddr() uint32 {
	return binary.BigEndian.Uint32(h.raw[16:20])
}

// SetDstAddr sets the destination IP address (network byte order).
func (h *IPv4Header) SetDstAddr(ip uint32) {
	binary.BigEndian.PutUint32(h.raw[16:20], ip)
}

// SrcIP returns the source IP as net.IP.
func (h *IPv4Header) SrcIP() net.IP {
	return net.IP(h.raw[12:16])
}

// DstIP returns the destination IP as net.IP.
func (h *IPv4Header) DstIP() net.IP {
	return net.IP(h.raw[16:20])
}

// IsLoopbackSrc returns true if the source IP is 127.x.x.x.
func (h *IPv4Header) IsLoopbackSrc() bool {
	return h.raw[12] == 127
}

// IsLoopbackDst returns true if the destination IP is 127.x.x.x.
func (h *IPv4Header) IsLoopbackDst() bool {
	return h.raw[16] == 127
}

// TCPHeader provides read/write access to TCP header fields in a raw packet buffer.
type TCPHeader struct {
	raw []byte
}

// SrcPort returns the TCP source port.
func (h *TCPHeader) SrcPort() uint16 {
	return binary.BigEndian.Uint16(h.raw[0:2])
}

// SetSrcPort sets the TCP source port.
func (h *TCPHeader) SetSrcPort(port uint16) {
	binary.BigEndian.PutUint16(h.raw[0:2], port)
}

// DstPort returns the TCP destination port.
func (h *TCPHeader) DstPort() uint16 {
	return binary.BigEndian.Uint16(h.raw[2:4])
}

// SetDstPort sets the TCP destination port.
func (h *TCPHeader) SetDstPort(port uint16) {
	binary.BigEndian.PutUint16(h.raw[2:4], port)
}

// DataOffset returns the TCP header length in bytes.
func (h *TCPHeader) DataOffset() int {
	return int(h.raw[12]>>4) * 4
}

// Flags returns the TCP flags byte.
func (h *TCPHeader) Flags() uint8 {
	return h.raw[13]
}

// IsSYN returns true if the SYN flag is set (and ACK is not).
func (h *TCPHeader) IsSYN() bool {
	return h.raw[13]&0x02 != 0 && h.raw[13]&0x10 == 0
}

// IsFIN returns true if the FIN flag is set.
func (h *TCPHeader) IsFIN() bool {
	return h.raw[13]&0x01 != 0
}

// IsRST returns true if the RST flag is set.
func (h *TCPHeader) IsRST() bool {
	return h.raw[13]&0x04 != 0
}

// ParsePacket attempts to parse buf as an IPv4+TCP packet.
// Returns the parsed headers and true on success, or nil, nil, false if
// the packet is not a valid IPv4 TCP packet.
func ParsePacket(buf []byte) (*IPv4Header, *TCPHeader, bool) {
	if len(buf) < 20 {
		return nil, nil, false
	}

	// Check IPv4 version
	version := buf[0] >> 4
	if version != 4 {
		return nil, nil, false
	}

	ipHeader := &IPv4Header{raw: buf}
	ihl := ipHeader.HeaderLen()
	if ihl < 20 || len(buf) < ihl {
		return nil, nil, false
	}

	// Check protocol is TCP (6)
	if ipHeader.Protocol() != 6 {
		return nil, nil, false
	}

	tcpStart := ihl
	if len(buf) < tcpStart+20 {
		return nil, nil, false
	}

	tcpHeader := &TCPHeader{raw: buf[tcpStart:]}
	return ipHeader, tcpHeader, true
}

// IP4to32 converts a net.IP to a network-byte-order uint32.
func IP4to32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip)
}

// IP32to4 converts a network-byte-order uint32 to a dotted IP string.
func IP32to4(ip uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		(ip>>24)&0xFF,
		(ip>>16)&0xFF,
		(ip>>8)&0xFF,
		ip&0xFF,
	)
}

// Localhost127 is 127.0.0.1 in network byte order.
const Localhost127 = 0x7F000001
