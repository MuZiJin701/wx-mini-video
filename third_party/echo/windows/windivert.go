//go:build windows

package windows

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	winDivertDLL *windows.LazyDLL

	procOpen          *windows.LazyProc
	procRecv          *windows.LazyProc
	procSend          *windows.LazyProc
	procClose         *windows.LazyProc
	procShutdown      *windows.LazyProc
	procSetParam      *windows.LazyProc
	procCalcChecksums *windows.LazyProc
)

func init() {
	initDLL("WinDivert.dll")
}

func initDLL(path string) {
	winDivertDLL = windows.NewLazyDLL(path)
	procOpen = winDivertDLL.NewProc("WinDivertOpen")
	procRecv = winDivertDLL.NewProc("WinDivertRecv")
	procSend = winDivertDLL.NewProc("WinDivertSend")
	procClose = winDivertDLL.NewProc("WinDivertClose")
	procShutdown = winDivertDLL.NewProc("WinDivertShutdown")
	procSetParam = winDivertDLL.NewProc("WinDivertSetParam")
	procCalcChecksums = winDivertDLL.NewProc("WinDivertHelperCalcChecksums")
}

// SetDLLPath sets the path to WinDivert.dll. Must be called before Start().
// The WinDivert64.sys driver file must be in the same directory as the DLL.
func SetDLLPath(dllPath string) {
	initDLL(dllPath)
}

// WinDivert layer constants.
const (
	LayerNetwork       = 0
	LayerNetworkForward = 1
)

// WinDivert shutdown constants.
const (
	ShutdownRecv = 0x1
	ShutdownSend = 0x2
	ShutdownBoth = 0x3
)

// WinDivert parameter constants.
const (
	ParamQueueLen  = 0
	ParamQueueTime = 1
	ParamQueueSize = 2
)

// Handle is a WinDivert handle.
type Handle uintptr

// Address matches the WINDIVERT_ADDRESS structure (64 bytes).
// We access fields through methods using unsafe pointer arithmetic
// to match the C bit-field layout.
type Address struct {
	Timestamp int64    // 8 bytes: packet timestamp
	Layer     uint8    // 1 byte: layer
	Event     uint8    // 1 byte: event
	Flags     uint8    // 1 byte: bit flags (Sniffed:1, Outbound:1, Loopback:1, Impostor:1, IPv6:1, IPChecksum:1, TCPChecksum:1, UDPChecksum:1)
	_pad1     uint8    // 1 byte padding
	DataLen   uint32   // 4 bytes: data length
	_         [48]byte // reserved/union (Network, Flow, Socket, Reflect, etc.)
}

// Outbound returns true if the packet was outbound.
func (a *Address) Outbound() bool {
	return a.Flags&0x02 != 0
}

// SetOutbound sets or clears the outbound flag.
func (a *Address) SetOutbound(outbound bool) {
	if outbound {
		a.Flags |= 0x02
	} else {
		a.Flags &^= 0x02
	}
}

// Loopback returns true if the packet is loopback.
func (a *Address) Loopback() bool {
	return a.Flags&0x04 != 0
}

// Open opens a WinDivert handle with the given filter.
func Open(filter string, layer int16, priority int16, flags uint64) (Handle, error) {
	if err := winDivertDLL.Load(); err != nil {
		return 0, fmt.Errorf("failed to load WinDivert.dll: %w", err)
	}

	filterPtr, err := windows.BytePtrFromString(filter)
	if err != nil {
		return 0, err
	}

	r1, _, e1 := procOpen.Call(
		uintptr(unsafe.Pointer(filterPtr)),
		uintptr(layer),
		uintptr(priority),
		uintptr(flags),
		uintptr(flags>>32),
	)

	h := Handle(r1)
	if h == Handle(^uintptr(0)) { // INVALID_HANDLE_VALUE
		if e1 != nil && e1 != windows.ERROR_SUCCESS {
			return 0, fmt.Errorf("WinDivertOpen failed: %w", e1)
		}
		return 0, fmt.Errorf("WinDivertOpen failed")
	}
	return h, nil
}

// Recv receives a packet from a WinDivert handle.
// Returns the number of bytes read and the packet address.
func Recv(h Handle, buf []byte) (int, *Address, error) {
	var addr Address
	var readLen uint32

	r1, _, e1 := procRecv.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&readLen)),
		uintptr(unsafe.Pointer(&addr)),
	)
	if r1 == 0 {
		if e1 != nil && e1 != windows.ERROR_SUCCESS {
			return 0, nil, fmt.Errorf("WinDivertRecv failed: %w", e1)
		}
		return 0, nil, fmt.Errorf("WinDivertRecv failed")
	}
	return int(readLen), &addr, nil
}

// RecvRaw receives a packet using pre-allocated (VirtualAlloc) buffer and address.
func RecvRaw(h Handle, buf []byte, addr *Address) (int, error) {
	var readLen uint32

	r1, _, e1 := procRecv.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&readLen)),
		uintptr(unsafe.Pointer(addr)),
	)
	if r1 == 0 {
		if e1 != nil && e1 != windows.ERROR_SUCCESS {
			return 0, fmt.Errorf("WinDivertRecv failed: %w", e1)
		}
		return 0, fmt.Errorf("WinDivertRecv failed")
	}
	return int(readLen), nil
}

// Send injects a packet through a WinDivert handle.
func Send(h Handle, buf []byte, addr *Address) error {
	var sendLen uint32
	r1, _, e1 := procSend.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&sendLen)),
		uintptr(unsafe.Pointer(addr)),
	)
	if r1 == 0 {
		if e1 != nil && e1 != windows.ERROR_SUCCESS {
			return fmt.Errorf("WinDivertSend failed: %w", e1)
		}
		return fmt.Errorf("WinDivertSend failed")
	}
	return nil
}

// CalcChecksums recalculates IP/TCP/UDP checksums for a packet.
func CalcChecksums(buf []byte, addr *Address, flags uint64) error {
	r1, _, e1 := procCalcChecksums.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(addr)),
		uintptr(flags),
		uintptr(flags>>32),
	)
	if r1 == 0 {
		if e1 != nil && e1 != windows.ERROR_SUCCESS {
			return fmt.Errorf("WinDivertHelperCalcChecksums failed: %w", e1)
		}
		return fmt.Errorf("WinDivertHelperCalcChecksums failed")
	}
	return nil
}

// Shutdown shuts down a WinDivert handle (unblocks Recv/Send).
func Shutdown(h Handle, how int) error {
	r1, _, e1 := procShutdown.Call(
		uintptr(h),
		uintptr(how),
	)
	if r1 == 0 {
		if e1 != nil && e1 != windows.ERROR_SUCCESS {
			return fmt.Errorf("WinDivertShutdown failed: %w", e1)
		}
		return fmt.Errorf("WinDivertShutdown failed")
	}
	return nil
}

// Close closes a WinDivert handle.
func Close(h Handle) error {
	r1, _, e1 := procClose.Call(uintptr(h))
	if r1 == 0 {
		if e1 != nil && e1 != windows.ERROR_SUCCESS {
			return fmt.Errorf("WinDivertClose failed: %w", e1)
		}
		return fmt.Errorf("WinDivertClose failed")
	}
	return nil
}

// SetParam sets a WinDivert parameter.
func SetParam(h Handle, param int, value uint64) error {
	r1, _, e1 := procSetParam.Call(
		uintptr(h),
		uintptr(param),
		uintptr(value),
		uintptr(value>>32),
	)
	if r1 == 0 {
		if e1 != nil && e1 != windows.ERROR_SUCCESS {
			return fmt.Errorf("WinDivertSetParam failed: %w", e1)
		}
		return fmt.Errorf("WinDivertSetParam failed")
	}
	return nil
}
