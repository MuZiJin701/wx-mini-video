//go:build windows

package windows

import (
	"sync"
	"time"
)

const (
	connHashSize     = 256
	connStaleTimeout = 60 * time.Second
)

// ConnEntry represents a tracked NAT connection.
type ConnEntry struct {
	SrcPort      uint16
	SrcIP        uint32 // network byte order
	OrigDestIP   uint32 // network byte order
	OrigDestPort uint16
	LastActivity time.Time
	next         *ConnEntry
}

type bucket struct {
	mu   sync.Mutex
	head *ConnEntry
}

// NATTable is a hash table tracking NAT-rewritten connections.
// It uses 256 buckets keyed by source port, with per-bucket locking.
type NATTable struct {
	buckets [connHashSize]bucket
}

// NewNATTable creates a new NAT connection tracking table.
func NewNATTable() *NATTable {
	return &NATTable{}
}

func hashPort(port uint16) int {
	return int(port) % connHashSize
}

// Add inserts or updates a connection entry keyed by srcPort.
func (t *NATTable) Add(srcPort uint16, srcIP, destIP uint32, destPort uint16) {
	idx := hashPort(srcPort)
	b := &t.buckets[idx]
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// Check if entry already exists, update in place
	for e := b.head; e != nil; e = e.next {
		if e.SrcPort == srcPort {
			e.SrcIP = srcIP
			e.OrigDestIP = destIP
			e.OrigDestPort = destPort
			e.LastActivity = now
			return
		}
	}

	// Insert new entry at head
	entry := &ConnEntry{
		SrcPort:      srcPort,
		SrcIP:        srcIP,
		OrigDestIP:   destIP,
		OrigDestPort: destPort,
		LastActivity: now,
	}
	entry.next = b.head
	b.head = entry
}

// Lookup returns the original destination IP and port for the given source port.
// Returns false if not found. Updates last activity time on hit.
func (t *NATTable) Lookup(srcPort uint16) (destIP uint32, destPort uint16, ok bool) {
	idx := hashPort(srcPort)
	b := &t.buckets[idx]
	b.mu.Lock()
	defer b.mu.Unlock()

	for e := b.head; e != nil; e = e.next {
		if e.SrcPort == srcPort {
			e.LastActivity = time.Now()
			return e.OrigDestIP, e.OrigDestPort, true
		}
	}
	return 0, 0, false
}

// IsTracked returns true if the source port has an active NAT entry.
func (t *NATTable) IsTracked(srcPort uint16) bool {
	idx := hashPort(srcPort)
	b := &t.buckets[idx]
	b.mu.Lock()
	defer b.mu.Unlock()

	for e := b.head; e != nil; e = e.next {
		if e.SrcPort == srcPort {
			return true
		}
	}
	return false
}

// Remove deletes the NAT entry for the given source port.
func (t *NATTable) Remove(srcPort uint16) {
	idx := hashPort(srcPort)
	b := &t.buckets[idx]
	b.mu.Lock()
	defer b.mu.Unlock()

	prev := (*ConnEntry)(nil)
	for e := b.head; e != nil; e = e.next {
		if e.SrcPort == srcPort {
			if prev == nil {
				b.head = e.next
			} else {
				prev.next = e.next
			}
			return
		}
		prev = e
	}
}

// Cleanup removes stale entries older than connStaleTimeout.
// Should be called periodically from a background goroutine.
func (t *NATTable) Cleanup() {
	now := time.Now()
	for i := 0; i < connHashSize; i++ {
		b := &t.buckets[i]
		b.mu.Lock()
		prev := (*ConnEntry)(nil)
		e := b.head
		for e != nil {
			if now.Sub(e.LastActivity) > connStaleTimeout {
				// Remove stale entry
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

// Clear removes all entries from the table.
func (t *NATTable) Clear() {
	for i := 0; i < connHashSize; i++ {
		b := &t.buckets[i]
		b.mu.Lock()
		b.head = nil
		b.mu.Unlock()
	}
}
