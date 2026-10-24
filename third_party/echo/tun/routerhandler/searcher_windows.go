//go:build windows

package routerhandler

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowsAF_INET  = 2
	windowsAF_INET6 = 23

	tcpTableOwnerPidBasic = 5
	udpTableOwnerPid      = 1
)

type mibTcpRowOwnerPid struct {
	DwState      uint32
	DwLocalAddr  uint32
	DwLocalPort  uint32
	DwRemoteAddr uint32
	DwRemotePort uint32
	DwOwningPid  uint32
}

type mibTcp6RowOwnerPid struct {
	LocalAddr    [16]byte
	DwScopeId    uint32
	DwState      uint32
	DwLocalPort  uint32
	DwRemotePort uint32
	DwRemoteAddr [16]byte
	DwOwningPid  uint32
}

type mibUdpRowOwnerPid struct {
	DwLocalAddr uint32
	DwLocalPort uint32
	DwOwningPid uint32
}

type mibUdp6RowOwnerPid struct {
	LocalAddr   [16]byte
	DwScopeId   uint32
	DwLocalPort uint32
	DwOwningPid uint32
}

var (
	modiphlpapi             *windows.LazyDLL
	procGetExtendedTcpTable *windows.LazyProc
	procGetExtendedUdpTable *windows.LazyProc
)

func loadExtendedTable() error {
	modiphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = modiphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = modiphlpapi.NewProc("GetExtendedUdpTable")
	return modiphlpapi.Load()
}

func callGetExtendedTcpTable(pTable *byte, pdwSize *uint32, bOrder bool, ulAf uint64, tableClass uint32, reserved uint64) error {
	var order uintptr
	if bOrder {
		order = 1
	}
	r0, _, _ := syscall.Syscall6(procGetExtendedTcpTable.Addr(), 6,
		uintptr(unsafe.Pointer(pTable)),
		uintptr(unsafe.Pointer(pdwSize)),
		order,
		uintptr(ulAf),
		uintptr(tableClass),
		uintptr(reserved),
	)
	if r0 != 0 {
		return syscall.Errno(r0)
	}
	return nil
}

func callGetExtendedUdpTable(pTable *byte, pdwSize *uint32, bOrder bool, ulAf uint64, tableClass uint32, reserved uint64) error {
	var order uintptr
	if bOrder {
		order = 1
	}
	r0, _, _ := syscall.Syscall6(procGetExtendedUdpTable.Addr(), 6,
		uintptr(unsafe.Pointer(pTable)),
		uintptr(unsafe.Pointer(pdwSize)),
		order,
		uintptr(ulAf),
		uintptr(tableClass),
		uintptr(reserved),
	)
	if r0 != 0 {
		return syscall.Errno(r0)
	}
	return nil
}

type extendedTableCall func(pTable *byte, pdwSize *uint32, bOrder bool, ulAf uint64, tableClass uint32, reserved uint64) error

func getExtendedTableRows[T any](call extendedTableCall, af uint64, tableClass uint32, _ T) ([]T, error) {
	var size uint32
	err := call(nil, &size, false, af, tableClass, 0)
	if err == nil || !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		return nil, os.NewSyscallError("getExtendedTable", err)
	}
	for {
		buf := make([]byte, size)
		err = call(&buf[0], &size, false, af, tableClass, 0)
		if err == nil {
			return unsafeSliceRows[T](buf, int(size)), nil
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			return nil, os.NewSyscallError("getExtendedTable", err)
		}
	}
}

func unsafeSliceRows[T any](buf []byte, bufSize int) []T {
	const headerSize = 4
	if len(buf) < headerSize {
		return nil
	}
	data := buf[headerSize:]
	count := len(data) / int(unsafe.Sizeof(*new(T)))
	_ = bufSize
	return unsafe.Slice((*T)(unsafe.Pointer(&data[0])), count)
}

func findPid(network string, source netip.AddrPort, dest netip.AddrPort) (uint32, error) {
	sourceAddr := source.Addr()
	sourcePort := source.Port()

	switch network {
	case "tcp":
		if sourceAddr.Is4() || sourceAddr.Is4In6() {
			rows, err := getExtendedTableRows(callGetExtendedTcpTable, windowsAF_INET, tcpTableOwnerPidBasic, mibTcpRowOwnerPid{})
			if err != nil {
				return 0, err
			}
			addr := ipv4ToUint32(sourceAddr.Unmap())
			port := portToUint32(sourcePort)
			for _, row := range rows {
				if row.DwLocalAddr == addr && row.DwLocalPort == port {
					return row.DwOwningPid, nil
				}
			}
			sPort := portToUint32(sourcePort)
			for _, row := range rows {
				if row.DwLocalPort == sPort {
					return row.DwOwningPid, nil
				}
			}
			return 0, ErrNotFound
		}
		rows, err := getExtendedTableRows(callGetExtendedTcpTable, windowsAF_INET6, tcpTableOwnerPidBasic, mibTcp6RowOwnerPid{})
		if err != nil {
			return 0, err
		}
		addr := sourceAddr.As16()
		port := portToUint32(sourcePort)
		for _, row := range rows {
			if row.LocalAddr == addr && row.DwLocalPort == port {
				return row.DwOwningPid, nil
			}
		}
		sPort := portToUint32(sourcePort)
		for _, row := range rows {
			if row.DwLocalPort == sPort {
				return row.DwOwningPid, nil
			}
		}
		return 0, ErrNotFound

	case "udp":
		if sourceAddr.Is4() || sourceAddr.Is4In6() {
			rows, err := getExtendedTableRows(callGetExtendedUdpTable, windowsAF_INET, udpTableOwnerPid, mibUdpRowOwnerPid{})
			if err != nil {
				return 0, err
			}
			addr := ipv4ToUint32(sourceAddr.Unmap())
			port := portToUint32(sourcePort)
			for _, row := range rows {
				if row.DwLocalAddr == addr && row.DwLocalPort == port {
					return row.DwOwningPid, nil
				}
			}
			for _, row := range rows {
				if row.DwLocalAddr == 0 && row.DwLocalPort == port {
					return row.DwOwningPid, nil
				}
			}
			return 0, ErrNotFound
		}
		rows, err := getExtendedTableRows(callGetExtendedUdpTable, windowsAF_INET6, udpTableOwnerPid, mibUdp6RowOwnerPid{})
		if err != nil {
			return 0, err
		}
		addr := sourceAddr.As16()
		port := portToUint32(sourcePort)
		for _, row := range rows {
			if row.LocalAddr == addr && row.DwLocalPort == port {
				return row.DwOwningPid, nil
			}
		}
		var unspec [16]byte
		for _, row := range rows {
			if row.LocalAddr == unspec && row.DwLocalPort == port {
				return row.DwOwningPid, nil
			}
		}
		return 0, ErrNotFound

	default:
		return 0, os.ErrInvalid
	}
}

func ipv4ToUint32(addr netip.Addr) uint32 {
	b := addr.As4()
	return binary.BigEndian.Uint32(b[:])
}

func portToUint32(port uint16) uint32 {
	return uint32(port>>8 | port<<8)
}

var _ Searcher = (*windowsSearcher)(nil)

type windowsSearcher struct{}

func NewSearcher(_ Config) (Searcher, error) {
	if err := loadExtendedTable(); err != nil {
		return nil, err
	}
	return &windowsSearcher{}, nil
}

func (s *windowsSearcher) Close() error { return nil }

func (s *windowsSearcher) FindProcessInfo(ctx context.Context, network string, source netip.AddrPort, dest netip.AddrPort) (*ConnectionOwner, error) {
	pid, err := findPid(network, source, dest)
	if err != nil {
		return nil, err
	}
	path, err := getProcessPath(pid)
	if err != nil {
		return &ConnectionOwner{ProcessID: pid, UserId: -1}, err
	}
	return &ConnectionOwner{ProcessID: pid, ProcessPath: path, UserId: -1}, nil
}

func getProcessPath(pid uint32) (string, error) {
	switch pid {
	case 0:
		return ":System Idle Process", nil
	case 4:
		return ":System", nil
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	size := uint32(syscall.MAX_LONG_PATH)
	buf := make([]uint16, syscall.MAX_LONG_PATH)
	err = windows.QueryFullProcessImageName(handle, 0, &buf[0], &size)
	if err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:size]), nil
}
