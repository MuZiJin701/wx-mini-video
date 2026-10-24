//go:build windows

package windows

import (
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	pidCacheSize  = 1024
	pidCacheTTL   = 1 * time.Second
	maxProcessName = 260
)

// TCP_TABLE_OWNER_PID_ALL for GetExtendedTcpTable
const tcpTableOwnerPidAll = 5

// MIB_TCPROW_OWNER_PID matches the Windows struct layout.
type mibTcpRowOwnerPid struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPid  uint32
}

var (
	iphlpapi              = windows.NewLazyDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")

	kernel32                     = windows.NewLazyDLL("kernel32.dll")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
)

// pidCacheEntry stores a cached PID lookup.
type pidCacheEntry struct {
	srcIP     uint32
	srcPort   uint16
	pid       uint32
	timestamp time.Time
	next      *pidCacheEntry
}

type pidBucket struct {
	mu   sync.Mutex
	head *pidCacheEntry
}

// ProcessResolver resolves connection source ports to process names.
type ProcessResolver struct {
	cache     [pidCacheSize]pidBucket
	selfPID   uint32
}

// NewProcessResolver creates a new process resolver.
func NewProcessResolver() *ProcessResolver {
	return &ProcessResolver{
		selfPID: uint32(windows.GetCurrentProcessId()),
	}
}

func pidHash(srcIP uint32, srcPort uint16) int {
	return int((srcIP ^ uint32(srcPort)) % pidCacheSize)
}

func (pr *ProcessResolver) getCachedPID(srcIP uint32, srcPort uint16) (uint32, bool) {
	idx := pidHash(srcIP, srcPort)
	b := &pr.cache[idx]
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	for e := b.head; e != nil; e = e.next {
		if e.srcIP == srcIP && e.srcPort == srcPort {
			if now.Sub(e.timestamp) < pidCacheTTL {
				return e.pid, true
			}
			// Expired, remove and re-lookup
			break
		}
	}
	return 0, false
}

func (pr *ProcessResolver) cachePID(srcIP uint32, srcPort uint16, pid uint32) {
	idx := pidHash(srcIP, srcPort)
	b := &pr.cache[idx]
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// Update existing
	for e := b.head; e != nil; e = e.next {
		if e.srcIP == srcIP && e.srcPort == srcPort {
			e.pid = pid
			e.timestamp = now
			return
		}
	}

	// Insert new
	entry := &pidCacheEntry{
		srcIP:     srcIP,
		srcPort:   srcPort,
		pid:       pid,
		timestamp: now,
	}
	entry.next = b.head
	b.head = entry
}

// GetPIDFromConnection looks up the PID that owns the given TCP connection.
func (pr *ProcessResolver) GetPIDFromConnection(srcIP uint32, srcPort uint16) uint32 {
	if pid, ok := pr.getCachedPID(srcIP, srcPort); ok {
		return pid
	}

	pid := pr.lookupTcpTable(srcIP, srcPort)
	if pid != 0 {
		pr.cachePID(srcIP, srcPort, pid)
	}
	return pid
}

func (pr *ProcessResolver) lookupTcpTable(srcIP uint32, srcPort uint16) uint32 {
	var size uint32
	// First call to get required size
	procGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, tcpTableOwnerPidAll, 0)
	if size == 0 {
		return 0
	}

	buf := make([]byte, size)
	ret, _, _ := procGetExtendedTcpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0,
		windows.AF_INET,
		tcpTableOwnerPidAll,
		0,
	)
	if ret != 0 {
		return 0
	}

	numEntries := *(*uint32)(unsafe.Pointer(&buf[0]))
	rowSize := int(unsafe.Sizeof(mibTcpRowOwnerPid{}))

	// Network byte order port: WinDivert gives us big-endian, but GetExtendedTcpTable
	// stores port as uint32 where the port is in the high 16 bits in network byte order.
	// We need to compare: table.LocalPort (stored as network-order in low 16 bits, upper 16 = 0)
	// with srcPort. The table stores port as (port_byte1 | port_byte2 << 8) in the low 16 bits.
	for i := uint32(0); i < numEntries; i++ {
		offset := 4 + int(i)*rowSize // skip dwNumEntries (4 bytes)
		if offset+rowSize > len(buf) {
			break
		}
		row := (*mibTcpRowOwnerPid)(unsafe.Pointer(&buf[offset]))
		// row.LocalPort has port in network byte order in the lower 16 bits
		tablePort := uint16(row.LocalPort & 0xFFFF)
		// Swap bytes to get host byte order and compare
		tablePortHost := (tablePort >> 8) | (tablePort << 8)
		if row.LocalAddr == srcIP && tablePortHost == srcPort {
			return row.OwningPid
		}
	}
	return 0
}

// GetProcessName returns the full executable path for the given PID.
func (pr *ProcessResolver) GetProcessName(pid uint32) (string, bool) {
	if pid == 0 {
		return "", false
	}
	if pid == 4 {
		return "System", true
	}

	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(handle)

	var buf [maxProcessName]uint16
	size := uint32(maxProcessName)
	ret, _, _ := procQueryFullProcessImageNameW.Call(
		uintptr(handle),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret == 0 {
		return "", false
	}
	return windows.UTF16ToString(buf[:size]), true
}

// IsSelf returns true if the PID matches this process.
func (pr *ProcessResolver) IsSelf(pid uint32) bool {
	return pid == pr.selfPID
}

// ClearCache removes all entries from the PID cache.
func (pr *ProcessResolver) ClearCache() {
	for i := 0; i < pidCacheSize; i++ {
		b := &pr.cache[i]
		b.mu.Lock()
		b.head = nil
		b.mu.Unlock()
	}
}

// CleanupCache removes expired entries from the PID cache.
func (pr *ProcessResolver) CleanupCache() {
	now := time.Now()
	for i := 0; i < pidCacheSize; i++ {
		b := &pr.cache[i]
		b.mu.Lock()
		prev := (*pidCacheEntry)(nil)
		e := b.head
		for e != nil {
			if now.Sub(e.timestamp) > 10*time.Second {
				if prev == nil {
					b.head = e.next
				} else {
					prev.next = e.next
				}
				e = e.next
			} else {
				prev = e
				e = e.next
			}
		}
		b.mu.Unlock()
	}
}
