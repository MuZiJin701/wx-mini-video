//go:build windows

package windows

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	maxPacketBuf      = 0xFFFF // 65535
	numPacketThreads  = 4
	defaultRelayPort  = 34010
	winDivertPriority = 123
	queueLength       = 8192
	queueTimeMs       = 2000
	cleanupInterval   = 30 * time.Second
)

// ConnectionCallback is called when a new connection is intercepted.
type ConnectionCallback func(pid uint32, processName string, srcIP, dstIP uint32, srcPort, dstPort uint16, action Action)

// Interceptor orchestrates the WinDivert-based per-process transparent proxy.
// It intercepts outbound TCP traffic from specified processes and redirects
// it through the local Relay server to the Echo HTTP proxy.
type Interceptor struct {
	echoAddr   string
	relayPort  uint16
	relayAddr  string

	rules    *RuleManager
	nat      *NATTable
	proc     *ProcessResolver
	relay    *Relay
	handle   Handle

	running    atomic.Bool
	dnsViaProxy atomic.Bool
	wg         sync.WaitGroup

	connCallback ConnectionCallback
	mu           sync.Mutex
}

// NewInterceptor creates a new Windows transparent proxy interceptor.
// echoAddr is the address of the Echo proxy (e.g., "127.0.0.1:8899").
func NewInterceptor(echoAddr string) *Interceptor {
	relayAddr := fmt.Sprintf("127.0.0.1:%d", defaultRelayPort)
	nat := NewNATTable()

	return &Interceptor{
		echoAddr:  echoAddr,
		relayPort: defaultRelayPort,
		relayAddr: relayAddr,
		rules:     NewRuleManager(),
		nat:       nat,
		proc:      NewProcessResolver(),
		relay:     NewRelay(relayAddr, echoAddr, nat),
	}
}

// AddRule adds a process interception rule and returns its ID.
func (i *Interceptor) AddRule(rule *ProcessRule) uint32 {
	return i.rules.AddRule(rule)
}

// RemoveRule removes a rule by ID.
func (i *Interceptor) RemoveRule(ruleID uint32) error {
	return i.rules.RemoveRule(ruleID)
}

// EnableRule enables a rule by ID.
func (i *Interceptor) EnableRule(ruleID uint32) error {
	return i.rules.EnableRule(ruleID)
}

// DisableRule disables a rule by ID.
func (i *Interceptor) DisableRule(ruleID uint32) error {
	return i.rules.DisableRule(ruleID)
}

// SetDNSViaProxy controls whether DNS (port 53) traffic is also proxied.
// By default, DNS traffic is allowed to pass through directly.
func (i *Interceptor) SetDNSViaProxy(enabled bool) {
	i.dnsViaProxy.Store(enabled)
}

// SetConnectionCallback sets an optional callback invoked when a new
// connection is intercepted and a rule decision is made.
func (i *Interceptor) SetConnectionCallback(cb ConnectionCallback) {
	i.mu.Lock()
	i.connCallback = cb
	i.mu.Unlock()
}

func (i *Interceptor) getCallback() ConnectionCallback {
	i.mu.Lock()
	cb := i.connCallback
	i.mu.Unlock()
	return cb
}

// Start begins intercepting traffic. Requires administrator privileges.
func (i *Interceptor) Start() error {
	if i.running.Load() {
		return fmt.Errorf("interceptor already running")
	}

	// 1. Start the TCP relay
	if err := i.relay.Start(); err != nil {
		return fmt.Errorf("failed to start relay: %w", err)
	}

	// 2. Brief pause to let relay bind
	time.Sleep(100 * time.Millisecond)

	// 3. Open WinDivert with TCP filter
	// Build a port-specific filter from rules to avoid capturing unrelated traffic
	// (e.g., RDP/SSH). Rules must be added before Start() for this to work.
	var filter string
	if portFilter, ok := i.rules.BuildPortFilter(); ok {
		// Narrow filter: only capture traffic to rule-specified ports + relay port
		filter = fmt.Sprintf(
			"tcp and ((outbound and (%s or tcp.SrcPort == %d)) or tcp.DstPort == %d)",
			portFilter, i.relayPort, i.relayPort,
		)
	} else {
		// Fallback: broad filter (may affect remote management connections)
		log.Printf("[interceptor] WARNING: rules have wildcard ports, using broad filter")
		filter = fmt.Sprintf(
			"tcp and (outbound or (tcp.DstPort == %d or tcp.SrcPort == %d))",
			i.relayPort, i.relayPort,
		)
	}
	h, err := Open(filter, LayerNetwork, winDivertPriority, 0)
	if err != nil {
		i.relay.Stop()
		return fmt.Errorf("WinDivert open failed (need admin?): %w", err)
	}
	i.handle = h

	// 4. Set queue parameters
	SetParam(h, ParamQueueLen, queueLength)
	SetParam(h, ParamQueueTime, queueTimeMs)

	i.running.Store(true)

	// 5. Start packet processing goroutines
	for t := 0; t < numPacketThreads; t++ {
		i.wg.Add(1)
		go i.packetProcessor()
	}

	// 6. Start NAT cleanup goroutine
	i.wg.Add(1)
	go i.cleanupLoop()

	log.Printf("[interceptor] started: relay=%s echo=%s filter=%q", i.relayAddr, i.echoAddr, filter)
	return nil
}

// Stop shuts down the interceptor and waits for all goroutines to finish.
func (i *Interceptor) Stop() error {
	if !i.running.Load() {
		return nil
	}

	i.running.Store(false)

	// Shutdown WinDivert to unblock Recv calls
	Shutdown(i.handle, ShutdownBoth)
	Close(i.handle)

	// Wait for all packet processors and cleanup to finish
	i.wg.Wait()

	// Stop the relay
	i.relay.Stop()

	// Clear state
	i.nat.Clear()
	i.proc.ClearCache()

	log.Printf("[interceptor] stopped")
	return nil
}

// processResult indicates what to do after processing a packet.
type processResult int

const (
	resultPassthrough processResult = iota // send unmodified
	resultModified                         // recalculate checksums and send
	resultDrop                             // discard the packet
)

func (i *Interceptor) packetProcessor() {
	defer i.wg.Done()

	// 使用 VirtualAlloc 分配缓冲区，确保内核驱动可以通过 WoW64 访问
	bufPtr, err := windows.VirtualAlloc(0, uintptr(maxPacketBuf), windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		log.Printf("[interceptor] VirtualAlloc for packet buffer failed: %v", err)
		return
	}
	defer windows.VirtualFree(bufPtr, 0, windows.MEM_RELEASE)
	buf := unsafe.Slice((*byte)(unsafe.Pointer(bufPtr)), maxPacketBuf)

	addrPtr, err := windows.VirtualAlloc(0, uintptr(unsafe.Sizeof(Address{})), windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		log.Printf("[interceptor] VirtualAlloc for address failed: %v", err)
		return
	}
	defer windows.VirtualFree(addrPtr, 0, windows.MEM_RELEASE)

	var pktCount uint64
	for i.running.Load() {
		addr := (*Address)(unsafe.Pointer(addrPtr))
		*addr = Address{} // 清零

		n, err := RecvRaw(i.handle, buf, addr)
		if err != nil {
			if !i.running.Load() {
				return
			}
			continue
		}

		pktCount++
		if pktCount <= 5 {
			log.Printf("[interceptor] pkt#%d len=%d flags=0x%02x outbound=%v", pktCount, n, addr.Flags, addr.Outbound())
		}

		packet := buf[:n]

		ipHdr, tcpHdr, ok := ParsePacket(packet)
		if !ok {
			if pktCount <= 5 {
				log.Printf("[interceptor] pkt#%d not a valid TCP packet, pass through", pktCount)
			}
			// Not a valid IPv4 TCP packet, pass through
			Send(i.handle, packet, addr)
			continue
		}

		if pktCount <= 5 {
			log.Printf("[interceptor] pkt#%d TCP src=%d dst=%d", pktCount, tcpHdr.SrcPort(), tcpHdr.DstPort())
		}

		var result processResult

		if addr.Outbound() {
			result = i.processOutbound(packet, ipHdr, tcpHdr, addr)
		} else {
			// Inbound: only process if destined for relay port
			if tcpHdr.DstPort() != i.relayPort {
				Send(i.handle, packet, addr)
				continue
			}
			result = resultModified
		}

		switch result {
		case resultDrop:
			// Silently discard
			continue
		case resultModified:
			CalcChecksums(packet, addr, 0)
		}
		Send(i.handle, packet, addr)
	}
}

func (i *Interceptor) processOutbound(packet []byte, ipHdr *IPv4Header, tcpHdr *TCPHeader, addr *Address) processResult {
	srcPort := tcpHdr.SrcPort()
	dstPort := tcpHdr.DstPort()
	srcIP := ipHdr.SrcAddr()
	dstIP := ipHdr.DstAddr()

	isLoopbackSrc := ipHdr.IsLoopbackSrc()
	isLoopbackDst := ipHdr.IsLoopbackDst()
	isBothLoopback := isLoopbackSrc && isLoopbackDst

	// Case 1: Response from relay port — reverse NAT
	if srcPort == i.relayPort {
		origIP, origPort, ok := i.nat.Lookup(dstPort)
		if !ok {
			return resultPassthrough
		}

		// Restore original source port to the original destination port
		tcpHdr.SetSrcPort(origPort)

		if isBothLoopback {
			// Loopback-to-loopback: restore source IP, keep outbound
			ipHdr.SetSrcAddr(origIP)
		} else {
			// Normal: swap IPs and flip direction
			ipHdr.SetSrcAddr(dstIP)
			ipHdr.SetDstAddr(srcIP)
			addr.SetOutbound(false)
		}

		// Remove NAT entry on FIN/RST
		if tcpHdr.IsFIN() || tcpHdr.IsRST() {
			i.nat.Remove(dstPort)
		}
		return resultModified
	}

	// Case 2: Already tracked connection — redirect to relay
	if i.nat.IsTracked(srcPort) {
		tcpHdr.SetDstPort(i.relayPort)

		if isBothLoopback {
			// Loopback: just change port, keep direction
			ipHdr.SetDstAddr(Localhost127BE())
		} else {
			// Swap IPs and flip direction
			ipHdr.SetSrcAddr(dstIP)
			ipHdr.SetDstAddr(srcIP)
			addr.SetOutbound(false)
		}

		if tcpHdr.IsFIN() || tcpHdr.IsRST() {
			i.nat.Remove(srcPort)
		}
		return resultModified
	}

	// Case 3: New connection — check rules
	if !i.rules.HasActiveRules() && i.getCallback() == nil {
		return resultPassthrough
	}

	// DNS bypass
	if dstPort == 53 && !i.dnsViaProxy.Load() {
		return resultPassthrough
	}

	isWebPort := dstPort == 80 || dstPort == 443

	// Look up PID and process name
	pid := i.proc.GetPIDFromConnection(srcIP, srcPort)
	if pid == 0 || i.proc.IsSelf(pid) {
		if isWebPort {
			log.Printf("[interceptor] port %d: PID lookup failed (pid=%d, isSelf=%v) srcPort=%d", dstPort, pid, pid != 0 && i.proc.IsSelf(pid), srcPort)
		}
		return resultPassthrough
	}

	processName, ok := i.proc.GetProcessName(pid)
	if !ok {
		if isWebPort {
			log.Printf("[interceptor] port %d: GetProcessName failed for PID=%d", dstPort, pid)
		}
		return resultPassthrough
	}

	if isWebPort {
		log.Printf("[interceptor] port %d: PID=%d Process=%s", dstPort, pid, processName)
	}

	action := i.rules.Match(processName, dstIP, dstPort)

	// Override: don't proxy loopback or broadcast/multicast destinations
	if action == ActionProxy {
		if isLoopbackDst {
			action = ActionDirect
		}
		if IsBroadcastOrMulticast(dstIP) {
			action = ActionDirect
		}
	}

	// Fire callback if set (only on SYN for new connections)
	if tcpHdr.IsSYN() {
		if cb := i.getCallback(); cb != nil {
			cb(pid, processName, srcIP, dstIP, srcPort, dstPort, action)
		}
	}

	switch action {
	case ActionBlock:
		return resultDrop

	case ActionDirect:
		return resultPassthrough

	case ActionProxy:
		// Add to NAT table
		i.nat.Add(srcPort, srcIP, dstIP, dstPort)

		// Redirect to relay
		tcpHdr.SetDstPort(i.relayPort)

		if isBothLoopback {
			ipHdr.SetDstAddr(Localhost127BE())
			// Keep outbound for loopback
		} else {
			// Swap IPs and flip direction
			ipHdr.SetSrcAddr(dstIP)
			ipHdr.SetDstAddr(srcIP)
			addr.SetOutbound(false)
		}
		return resultModified
	}

	return resultPassthrough
}

func (i *Interceptor) cleanupLoop() {
	defer i.wg.Done()

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !i.running.Load() {
				return
			}
			i.nat.Cleanup()
			i.proc.CleanupCache()
		}
	}
}

// Localhost127BE returns 127.0.0.1 in big-endian (network byte order).
func Localhost127BE() uint32 {
	var b [4]byte
	b[0] = 127
	b[1] = 0
	b[2] = 0
	b[3] = 1
	return binary.BigEndian.Uint32(b[:])
}
