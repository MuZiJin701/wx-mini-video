package routerhandler

import (
	"context"
	"errors"
	"net/netip"
	"os/user"
	"strconv"
)

// ConnectionOwner represents the process that owns a network connection.
type ConnectionOwner struct {
	ProcessID           uint32
	UserId              int32
	UserName            string
	ProcessPath         string
	AndroidPackageNames []string
}

// Searcher is the interface for finding which process owns a network connection.
type Searcher interface {
	FindProcessInfo(ctx context.Context, network string, source netip.AddrPort, dest netip.AddrPort) (*ConnectionOwner, error)
	Close() error
}

// ErrNotFound is returned when no process can be found for a connection.
var ErrNotFound = errors.New("process not found")

// Config holds optional settings for creating a Searcher.
type Config struct {
	Logger func(format string, args ...interface{})
}

// FindProcessInfo is a convenience wrapper that looks up the connection owner
// and fills in the UserName field from the OS if it's empty but UserId is set.
func FindProcessInfo(s Searcher, ctx context.Context, network string, source, dest netip.AddrPort) (*ConnectionOwner, error) {
	info, err := s.FindProcessInfo(ctx, network, source, dest)
	if err != nil {
		return nil, err
	}
	if info.UserId != -1 && info.UserName == "" {
		osUser, _ := user.LookupId(strconv.Itoa(int(info.UserId)))
		if osUser != nil {
			info.UserName = osUser.Username
		}
	}
	return info, nil
}
