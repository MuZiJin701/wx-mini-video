//go:build linux && !android

package routerhandler

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
)

const (
	sizeOfSocketDiagRequestData = 56
	sizeOfSocketDiagRequest     = syscall.SizeofNlMsghdr + sizeOfSocketDiagRequestData
	socketDiagResponseMinSize   = 72
	socketDiagByFamily          = 20
	pathProc                    = "/proc"
)

type socketDiagConn struct {
	access   sync.Mutex
	family   uint8
	protocol uint8
	fd       int
}

func newSocketDiagConn(family, protocol uint8) *socketDiagConn {
	return &socketDiagConn{
		family:   family,
		protocol: protocol,
		fd:       -1,
	}
}

func socketDiagConnIndex(family, protocol uint8) int {
	index := 0
	if protocol == syscall.IPPROTO_UDP {
		index += 2
	}
	if family == syscall.AF_INET6 {
		index++
	}
	return index
}

func socketDiagSettings(network string, source netip.AddrPort) (family, protocol uint8, err error) {
	switch network {
	case "tcp":
		protocol = syscall.IPPROTO_TCP
	case "udp":
		protocol = syscall.IPPROTO_UDP
	default:
		return 0, 0, os.ErrInvalid
	}
	switch {
	case source.Addr().Is4():
		family = syscall.AF_INET
	case source.Addr().Is6():
		family = syscall.AF_INET6
	default:
		return 0, 0, os.ErrInvalid
	}
	return family, protocol, nil
}

func (c *socketDiagConn) Close() error {
	c.access.Lock()
	defer c.access.Unlock()
	return c.closeLocked()
}

func (c *socketDiagConn) query(source netip.AddrPort, destination netip.AddrPort) (inode, uid uint32, err error) {
	c.access.Lock()
	defer c.access.Unlock()
	request := packSocketDiagRequest(c.family, c.protocol, source, destination, false)
	for attempt := 0; attempt < 2; attempt++ {
		if err = c.ensureOpenLocked(); err != nil {
			return 0, 0, fmt.Errorf("dial netlink: %w", err)
		}
		inode, uid, err = querySocketDiag(c.fd, request)
		if err == nil || errors.Is(err, ErrNotFound) {
			return inode, uid, err
		}
		if !shouldRetrySocketDiag(err) {
			return 0, 0, err
		}
		_ = c.closeLocked()
	}
	return 0, 0, err
}

func querySocketDiagOnce(family, protocol uint8, source netip.AddrPort) (inode, uid uint32, err error) {
	fd, err := openSocketDiag()
	if err != nil {
		return 0, 0, fmt.Errorf("dial netlink: %w", err)
	}
	defer syscall.Close(fd)
	return querySocketDiag(fd, packSocketDiagRequest(family, protocol, source, netip.AddrPort{}, true))
}

func (c *socketDiagConn) ensureOpenLocked() error {
	if c.fd != -1 {
		return nil
	}
	fd, err := openSocketDiag()
	if err != nil {
		return err
	}
	c.fd = fd
	return nil
}

func openSocketDiag() (int, error) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, syscall.NETLINK_INET_DIAG)
	if err != nil {
		return -1, err
	}
	timeout := &syscall.Timeval{Usec: 100}
	if err = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, timeout); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	if err = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, timeout); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	if err = syscall.Connect(fd, &syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Pid:    0,
		Groups: 0,
	}); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	return fd, nil
}

func (c *socketDiagConn) closeLocked() error {
	if c.fd == -1 {
		return nil
	}
	err := syscall.Close(c.fd)
	c.fd = -1
	return err
}

func packSocketDiagRequest(family, protocol byte, source netip.AddrPort, destination netip.AddrPort, dump bool) []byte {
	request := make([]byte, sizeOfSocketDiagRequest)

	binary.LittleEndian.PutUint32(request[0:4], sizeOfSocketDiagRequest)
	binary.LittleEndian.PutUint16(request[4:6], socketDiagByFamily)
	flags := uint16(syscall.NLM_F_REQUEST)
	if dump {
		flags |= syscall.NLM_F_DUMP
	}
	binary.LittleEndian.PutUint16(request[6:8], flags)
	binary.LittleEndian.PutUint32(request[8:12], 0)
	binary.LittleEndian.PutUint32(request[12:16], 0)

	request[16] = family
	request[17] = protocol
	request[18] = 0
	request[19] = 0
	if dump {
		binary.LittleEndian.PutUint32(request[20:24], 0xFFFFFFFF)
	}
	requestSource := source
	requestDestination := destination
	if protocol == syscall.IPPROTO_UDP && !dump && destination.IsValid() {
		requestSource, requestDestination = destination, source
	}
	binary.BigEndian.PutUint16(request[24:26], requestSource.Port())
	binary.BigEndian.PutUint16(request[26:28], requestDestination.Port())
	if family == syscall.AF_INET6 {
		copy(request[28:44], requestSource.Addr().AsSlice())
		if requestDestination.IsValid() {
			copy(request[44:60], requestDestination.Addr().AsSlice())
		}
	} else {
		copy(request[28:32], requestSource.Addr().AsSlice())
		if requestDestination.IsValid() {
			copy(request[44:48], requestDestination.Addr().AsSlice())
		}
	}
	binary.LittleEndian.PutUint32(request[60:64], 0)
	binary.LittleEndian.PutUint64(request[64:72], 0xFFFFFFFFFFFFFFFF)
	return request
}

func querySocketDiag(fd int, request []byte) (inode, uid uint32, err error) {
	_, err = syscall.Write(fd, request)
	if err != nil {
		return 0, 0, fmt.Errorf("write netlink request: %w", err)
	}
	buffer := make([]byte, 64<<10)
	n, err := syscall.Read(fd, buffer)
	if err != nil {
		return 0, 0, fmt.Errorf("read netlink response: %w", err)
	}
	messages, err := syscall.ParseNetlinkMessage(buffer[:n])
	if err != nil {
		return 0, 0, fmt.Errorf("parse netlink message: %w", err)
	}
	return unpackSocketDiagMessages(messages)
}

func unpackSocketDiagMessages(messages []syscall.NetlinkMessage) (inode, uid uint32, err error) {
	for _, message := range messages {
		switch message.Header.Type {
		case syscall.NLMSG_DONE:
			continue
		case syscall.NLMSG_ERROR:
			if err = unpackSocketDiagError(&message); err != nil {
				return 0, 0, err
			}
		case socketDiagByFamily:
			inode, uid = unpackSocketDiagResponse(&message)
			if inode != 0 || uid != 0 {
				return inode, uid, nil
			}
		}
	}
	return 0, 0, ErrNotFound
}

func unpackSocketDiagResponse(msg *syscall.NetlinkMessage) (inode, uid uint32) {
	if len(msg.Data) < socketDiagResponseMinSize {
		return 0, 0
	}
	uid = binary.LittleEndian.Uint32(msg.Data[64:68])
	inode = binary.LittleEndian.Uint32(msg.Data[68:72])
	return inode, uid
}

func unpackSocketDiagError(msg *syscall.NetlinkMessage) error {
	if len(msg.Data) < 4 {
		return errors.New("netlink message: NLMSG_ERROR")
	}
	errno := int32(binary.LittleEndian.Uint32(msg.Data[:4]))
	if errno == 0 {
		return nil
	}
	if errno < 0 {
		errno = -errno
	}
	sysErr := syscall.Errno(errno)
	switch sysErr {
	case syscall.ENOENT, syscall.ESRCH:
		return ErrNotFound
	default:
		return fmt.Errorf("netlink message: %s", sysErr)
	}
}

func shouldRetrySocketDiag(err error) bool {
	return err != nil && !errors.Is(err, ErrNotFound)
}

type uidProcessPathCache struct {
	mu      sync.Mutex
	entries map[uint32]*uidProcessPaths
	ttl     time.Duration
	lruHead *uidCacheNode
	lruTail *uidCacheNode
	lruMap  map[uint32]*uidCacheNode
	maxSize int
}

type uidCacheNode struct {
	uid  uint32
	prev *uidCacheNode
	next *uidCacheNode
}

type uidProcessPaths struct {
	entries map[uint32]string
}

func newUIDProcessPathCache(ttl time.Duration) *uidProcessPathCache {
	return &uidProcessPathCache{
		entries: make(map[uint32]*uidProcessPaths),
		ttl:     ttl,
		lruMap:  make(map[uint32]*uidCacheNode),
		maxSize: 64,
	}
}

func (c *uidProcessPathCache) findProcessPath(targetInode, uid uint32) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cached, ok := c.entries[uid]; ok {
		if processPath, found := cached.entries[targetInode]; found {
			c.touchLRULocked(uid)
			return processPath, nil
		}
	}

	processPaths, err := buildProcessPathByUIDCache(uid)
	if err != nil {
		return "", err
	}

	for len(c.entries) >= c.maxSize && c.lruTail != nil {
		delete(c.entries, c.lruTail.uid)
		c.removeLRUNodeLocked(c.lruTail)
	}

	c.entries[uid] = &uidProcessPaths{entries: processPaths}
	c.addLRULocked(uid)

	processPath, found := processPaths[targetInode]
	if !found {
		return "", fmt.Errorf("process of uid(%d), inode(%d) not found", uid, targetInode)
	}
	return processPath, nil
}

func (c *uidProcessPathCache) touchLRULocked(uid uint32) {
	if node, ok := c.lruMap[uid]; ok {
		c.removeLRUNodeLocked(node)
		c.addLRUNodeLocked(node)
	}
}

func (c *uidProcessPathCache) addLRULocked(uid uint32) {
	node := &uidCacheNode{uid: uid}
	c.addLRUNodeLocked(node)
}

func (c *uidProcessPathCache) addLRUNodeLocked(node *uidCacheNode) {
	node.next = c.lruHead
	node.prev = nil
	if c.lruHead != nil {
		c.lruHead.prev = node
	}
	c.lruHead = node
	if c.lruTail == nil {
		c.lruTail = node
	}
	c.lruMap[node.uid] = node
}

func (c *uidProcessPathCache) removeLRUNodeLocked(node *uidCacheNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		c.lruHead = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		c.lruTail = node.prev
	}
	delete(c.lruMap, node.uid)
}

func buildProcessPathByUIDCache(uid uint32) (map[uint32]string, error) {
	files, err := os.ReadDir(pathProc)
	if err != nil {
		return nil, err
	}
	buffer := make([]byte, syscall.PathMax)
	processPaths := make(map[uint32]string)
	for _, file := range files {
		if !file.IsDir() || !isPid(file.Name()) {
			continue
		}
		info, err := file.Info()
		if err != nil {
			if isIgnorableProcError(err) {
				continue
			}
			return nil, err
		}
		if info.Sys().(*syscall.Stat_t).Uid != uid {
			continue
		}
		processPath := filepath.Join(pathProc, file.Name())
		fdPath := filepath.Join(processPath, "fd")
		exePath, err := os.Readlink(filepath.Join(processPath, "exe"))
		if err != nil {
			if isIgnorableProcError(err) {
				continue
			}
			return nil, err
		}
		fds, err := os.ReadDir(fdPath)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			n, err := syscall.Readlink(filepath.Join(fdPath, fd.Name()), buffer)
			if err != nil {
				continue
			}
			inode, ok := parseSocketInode(buffer[:n])
			if !ok {
				continue
			}
			if _, loaded := processPaths[inode]; !loaded {
				processPaths[inode] = exePath
			}
		}
	}
	return processPaths, nil
}

func isIgnorableProcError(err error) bool {
	return os.IsNotExist(err) || os.IsPermission(err)
}

func parseSocketInode(link []byte) (uint32, bool) {
	const socketPrefix = "socket:["
	if len(link) <= len(socketPrefix) || string(link[:len(socketPrefix)]) != socketPrefix || link[len(link)-1] != ']' {
		return 0, false
	}
	var inode uint64
	for _, char := range link[len(socketPrefix) : len(link)-1] {
		if char < '0' || char > '9' {
			return 0, false
		}
		inode = inode*10 + uint64(char-'0')
		if inode > uint64(^uint32(0)) {
			return 0, false
		}
	}
	return uint32(inode), true
}

func isPid(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil && strings.IndexFunc(s, func(r rune) bool {
		return !unicode.IsDigit(r)
	}) == -1
}

var _ Searcher = (*linuxSearcher)(nil)

type linuxSearcher struct {
	logger           func(format string, args ...interface{})
	diagConns        [4]*socketDiagConn
	processPathCache *uidProcessPathCache
}

func NewSearcher(config Config) (Searcher, error) {
	searcher := &linuxSearcher{
		logger:           config.Logger,
		processPathCache: newUIDProcessPathCache(time.Second),
	}
	for _, family := range []uint8{syscall.AF_INET, syscall.AF_INET6} {
		for _, protocol := range []uint8{syscall.IPPROTO_TCP, syscall.IPPROTO_UDP} {
			searcher.diagConns[socketDiagConnIndex(family, protocol)] = newSocketDiagConn(family, protocol)
		}
	}
	return searcher, nil
}

func (s *linuxSearcher) Close() error {
	var errs []error
	for _, conn := range s.diagConns {
		if conn == nil {
			continue
		}
		errs = append(errs, conn.Close())
	}
	return errors.Join(errs...)
}

func (s *linuxSearcher) FindProcessInfo(ctx context.Context, network string, source netip.AddrPort, destination netip.AddrPort) (*ConnectionOwner, error) {
	inode, uid, err := s.resolveSocketByNetlink(network, source, destination)
	if err != nil {
		return nil, err
	}
	processInfo := &ConnectionOwner{
		UserId: int32(uid),
	}
	processPath, err := s.processPathCache.findProcessPath(inode, uid)
	if err != nil {
		if s.logger != nil {
			s.logger("find process path: %v", err)
		}
	} else {
		processInfo.ProcessPath = processPath
	}
	return processInfo, nil
}

func (s *linuxSearcher) resolveSocketByNetlink(network string, source netip.AddrPort, destination netip.AddrPort) (inode, uid uint32, err error) {
	family, protocol, err := socketDiagSettings(network, source)
	if err != nil {
		return 0, 0, err
	}
	conn := s.diagConns[socketDiagConnIndex(family, protocol)]
	if conn == nil {
		return 0, 0, fmt.Errorf("missing socket diag connection for family=%d protocol=%d", family, protocol)
	}
	if destination.IsValid() && source.Addr().BitLen() == destination.Addr().BitLen() {
		inode, uid, err = conn.query(source, destination)
		if err == nil {
			return inode, uid, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return 0, 0, err
		}
	}
	return querySocketDiagOnce(family, protocol, source)
}
