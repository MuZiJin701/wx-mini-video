package routerhandler

import (
	"context"
	"net/netip"
	"sync"
	"time"
)

// cacheEntry holds a cached result or error for a connection lookup.
type cacheEntry struct {
	owner *ConnectionOwner
	err   error
}

// cacheKey uniquely identifies a connection for caching purposes.
type cacheKey struct {
	network string
	source  netip.AddrPort
	dest    netip.AddrPort
}

// lruCache is a simple LRU cache.
type lruCache[K comparable, V any] struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	items    map[K]*lruNode[K, V]
	head     *lruNode[K, V]
	tail     *lruNode[K, V]
}

type lruNode[K comparable, V any] struct {
	key       K
	value     V
	expiresAt time.Time
	prev      *lruNode[K, V]
	next      *lruNode[K, V]
}

func newLRUCache[K comparable, V any](capacity int, ttl time.Duration) *lruCache[K, V] {
	return &lruCache[K, V]{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[K]*lruNode[K, V]),
	}
}

func (c *lruCache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if node, ok := c.items[key]; ok {
		if time.Now().Before(node.expiresAt) {
			c.moveToFront(node)
			return node.value, true
		}
		c.remove(node)
	}
	var zero V
	return zero, false
}

func (c *lruCache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if node, ok := c.items[key]; ok {
		node.value = value
		node.expiresAt = time.Now().Add(c.ttl)
		c.moveToFront(node)
		return
	}
	for len(c.items) >= c.capacity {
		c.remove(c.tail)
	}
	node := &lruNode[K, V]{
		key:       key,
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.items[key] = node
	if c.head != nil {
		c.head.prev = node
		node.next = c.head
	}
	c.head = node
	if c.tail == nil {
		c.tail = node
	}
}

func (c *lruCache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *lruCache[K, V]) moveToFront(node *lruNode[K, V]) {
	if node == c.head {
		return
	}
	if node.prev != nil {
		node.prev.next = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	}
	if node == c.tail {
		c.tail = node.prev
	}
	node.prev = nil
	node.next = c.head
	if c.head != nil {
		c.head.prev = node
	}
	c.head = node
}

func (c *lruCache[K, V]) remove(node *lruNode[K, V]) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		c.head = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		c.tail = node.prev
	}
	delete(c.items, node.key)
}

// CachedSearcher wraps a Searcher with an LRU cache for connection lookups.
type CachedSearcher struct {
	inner Searcher
	cache *lruCache[cacheKey, cacheEntry]
}

// NewCachedSearcher creates a CachedSearcher with the given capacity and TTL.
func NewCachedSearcher(inner Searcher, size int, ttl time.Duration) *CachedSearcher {
	return &CachedSearcher{
		inner: inner,
		cache: newLRUCache[cacheKey, cacheEntry](size, ttl),
	}
}

func (c *CachedSearcher) FindProcessInfo(ctx context.Context, network string, source, dest netip.AddrPort) (*ConnectionOwner, error) {
	key := cacheKey{network: network, source: source, dest: dest}
	if entry, ok := c.cache.Get(key); ok {
		return entry.owner, entry.err
	}
	owner, err := c.inner.FindProcessInfo(ctx, network, source, dest)
	c.cache.Set(key, cacheEntry{owner: owner, err: err})
	return owner, err
}

func (c *CachedSearcher) Close() error {
	return c.inner.Close()
}
