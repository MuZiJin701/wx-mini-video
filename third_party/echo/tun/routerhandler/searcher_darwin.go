//go:build darwin

package routerhandler

import (
	"context"
	"encoding/binary"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	darwinSnapshotTTL = 200 * time.Millisecond

	darwinXinpgenSize        = 24
	darwinXsocketOffset      = 104
	darwinXinpcbForeignPort  = 16
	darwinXinpcbLocalPort    = 18
	darwinXinpcbVFlag        = 44
	darwinXinpcbForeignAddr  = 48
	darwinXinpcbLocalAddr    = 64
	darwinXinpcbIPv4Addr     = 12
	darwinXsocketUID         = 64
	darwinXsocketLastPID     = 68
	darwinTCPExtraStructSize = 208
)

var structSize = func() int {
	value, _ := syscall.Sysctl("kern.osrelease")
	major, _, _ := strings.Cut(value, ".")
	n, _ := strconv.ParseInt(major, 10, 64)
	if n >= 22 {
		return 408
	}
	return 384
}()

type darwinConnectionEntry struct {
	localAddr  netip.Addr
	remoteAddr netip.Addr
	localPort  uint16
	remotePort uint16
	pid        uint32
	uid        int32
}

type darwinConnectionMatchKind uint8

const (
	darwinConnectionMatchExact darwinConnectionMatchKind = iota
	darwinConnectionMatchLocalFallback
	darwinConnectionMatchWildcardFallback
)

type darwinSnapshot struct {
	createdAt time.Time
	entries   []darwinConnectionEntry
}

type darwinConnectionFinder struct {
	access    sync.Mutex
	ttl       time.Duration
	snapshots map[string]darwinSnapshot
}

var sharedDarwinConnectionFinder = &darwinConnectionFinder{
	ttl:       darwinSnapshotTTL,
	snapshots: make(map[string]darwinSnapshot),
}

func (f *darwinConnectionFinder) find(network string, source netip.AddrPort, destination netip.AddrPort) (*ConnectionOwner, error) {
	source = normalizeDarwinAddrPort(source)
	destination = normalizeDarwinAddrPort(destination)
	var lastOwner *ConnectionOwner
	for attempt := 0; attempt < 2; attempt++ {
		snapshot, fromCache, err := f.loadSnapshot(network, attempt > 0)
		if err != nil {
			return nil, err
		}
		entry, matchKind, err := matchDarwinConnectionEntry(snapshot.entries, network, source, destination)
		if err != nil {
			if err == ErrNotFound && fromCache {
				continue
			}
			return nil, err
		}
		if fromCache && matchKind != darwinConnectionMatchExact {
			continue
		}
		owner := &ConnectionOwner{UserId: entry.uid}
		lastOwner = owner
		if entry.pid == 0 {
			return owner, nil
		}
		processPath, err := getExecPathFromPID(entry.pid)
		if err == nil {
			owner.ProcessPath = processPath
			return owner, nil
		}
		if fromCache {
			continue
		}
		return owner, nil
	}
	if lastOwner != nil {
		return lastOwner, nil
	}
	return nil, ErrNotFound
}

func (f *darwinConnectionFinder) loadSnapshot(network string, forceRefresh bool) (darwinSnapshot, bool, error) {
	f.access.Lock()
	defer f.access.Unlock()
	if !forceRefresh {
		if snapshot, loaded := f.snapshots[network]; loaded && time.Since(snapshot.createdAt) < f.ttl {
			return snapshot, true, nil
		}
	}
	snapshot, err := buildDarwinSnapshot(network)
	if err != nil {
		return darwinSnapshot{}, false, err
	}
	f.snapshots[network] = snapshot
	return snapshot, false, nil
}

func buildDarwinSnapshot(network string) (darwinSnapshot, error) {
	spath, itemSize, err := darwinSnapshotSettings(network)
	if err != nil {
		return darwinSnapshot{}, err
	}
	value, err := unix.SysctlRaw(spath)
	if err != nil {
		return darwinSnapshot{}, err
	}
	return darwinSnapshot{
		createdAt: time.Now(),
		entries:   parseDarwinSnapshot(value, itemSize),
	}, nil
}

func darwinSnapshotSettings(network string) (string, int, error) {
	itemSize := structSize
	switch network {
	case "tcp":
		return "net.inet.tcp.pcblist_n", itemSize + darwinTCPExtraStructSize, nil
	case "udp":
		return "net.inet.udp.pcblist_n", itemSize, nil
	default:
		return "", 0, os.ErrInvalid
	}
}

func parseDarwinSnapshot(buf []byte, itemSize int) []darwinConnectionEntry {
	entries := make([]darwinConnectionEntry, 0, (len(buf)-darwinXinpgenSize)/itemSize)
	for i := darwinXinpgenSize; i+itemSize <= len(buf); i += itemSize {
		inp := i
		so := i + darwinXsocketOffset
		entry, ok := parseDarwinConnectionEntry(buf[inp:so], buf[so:so+structSize-darwinXsocketOffset])
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func parseDarwinConnectionEntry(inp []byte, so []byte) (darwinConnectionEntry, bool) {
	if len(inp) < darwinXsocketOffset || len(so) < structSize-darwinXsocketOffset {
		return darwinConnectionEntry{}, false
	}
	entry := darwinConnectionEntry{
		remotePort: binary.BigEndian.Uint16(inp[darwinXinpcbForeignPort : darwinXinpcbForeignPort+2]),
		localPort:  binary.BigEndian.Uint16(inp[darwinXinpcbLocalPort : darwinXinpcbLocalPort+2]),
		pid:        binary.LittleEndian.Uint32(so[darwinXsocketLastPID : darwinXsocketLastPID+4]),
		uid:        int32(binary.LittleEndian.Uint32(so[darwinXsocketUID : darwinXsocketUID+4])),
	}
	flag := inp[darwinXinpcbVFlag]
	switch {
	case flag&0x1 != 0:
		entry.remoteAddr = netip.AddrFrom4([4]byte(inp[darwinXinpcbForeignAddr+darwinXinpcbIPv4Addr : darwinXinpcbForeignAddr+darwinXinpcbIPv4Addr+4]))
		entry.localAddr = netip.AddrFrom4([4]byte(inp[darwinXinpcbLocalAddr+darwinXinpcbIPv4Addr : darwinXinpcbLocalAddr+darwinXinpcbIPv4Addr+4]))
		return entry, true
	case flag&0x2 != 0:
		entry.remoteAddr = netip.AddrFrom16([16]byte(inp[darwinXinpcbForeignAddr : darwinXinpcbForeignAddr+16]))
		entry.localAddr = netip.AddrFrom16([16]byte(inp[darwinXinpcbLocalAddr : darwinXinpcbLocalAddr+16]))
		return entry, true
	default:
		return darwinConnectionEntry{}, false
	}
}

func matchDarwinConnectionEntry(entries []darwinConnectionEntry, network string, source netip.AddrPort, destination netip.AddrPort) (darwinConnectionEntry, darwinConnectionMatchKind, error) {
	sourceAddr := source.Addr()
	if !sourceAddr.IsValid() {
		return darwinConnectionEntry{}, darwinConnectionMatchExact, os.ErrInvalid
	}
	var localFallback darwinConnectionEntry
	var hasLocalFallback bool
	var wildcardFallback darwinConnectionEntry
	var hasWildcardFallback bool
	for _, entry := range entries {
		if entry.localPort != source.Port() || sourceAddr.BitLen() != entry.localAddr.BitLen() {
			continue
		}
		if entry.localAddr == sourceAddr && destination.IsValid() && entry.remotePort == destination.Port() && entry.remoteAddr == destination.Addr() {
			return entry, darwinConnectionMatchExact, nil
		}
		if !destination.IsValid() && entry.localAddr == sourceAddr {
			return entry, darwinConnectionMatchExact, nil
		}
		if network != "udp" {
			continue
		}
		if !hasLocalFallback && entry.localAddr == sourceAddr {
			hasLocalFallback = true
			localFallback = entry
		}
		if !hasWildcardFallback && entry.localAddr.IsUnspecified() {
			hasWildcardFallback = true
			wildcardFallback = entry
		}
	}
	if hasLocalFallback {
		return localFallback, darwinConnectionMatchLocalFallback, nil
	}
	if hasWildcardFallback {
		return wildcardFallback, darwinConnectionMatchWildcardFallback, nil
	}
	return darwinConnectionEntry{}, darwinConnectionMatchExact, ErrNotFound
}

func normalizeDarwinAddrPort(addrPort netip.AddrPort) netip.AddrPort {
	if !addrPort.IsValid() {
		return addrPort
	}
	return netip.AddrPortFrom(addrPort.Addr().Unmap(), addrPort.Port())
}

func getExecPathFromPID(pid uint32) (string, error) {
	const (
		procpidpathinfo     = 0xb
		procpidpathinfosize = 1024
		proccallnumpidinfo  = 0x2
	)
	buf := make([]byte, procpidpathinfosize)
	_, _, errno := syscall.Syscall6(
		syscall.SYS_PROC_INFO,
		proccallnumpidinfo,
		uintptr(pid),
		procpidpathinfo,
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		procpidpathinfosize)
	if errno != 0 {
		return "", errno
	}
	return unix.ByteSliceToString(buf), nil
}

var _ Searcher = (*darwinSearcher)(nil)

type darwinSearcher struct{}

func NewSearcher(_ Config) (Searcher, error) {
	return &darwinSearcher{}, nil
}

func (d *darwinSearcher) Close() error { return nil }

func (d *darwinSearcher) FindProcessInfo(ctx context.Context, network string, source netip.AddrPort, dest netip.AddrPort) (*ConnectionOwner, error) {
	return sharedDarwinConnectionFinder.find(network, source, dest)
}
