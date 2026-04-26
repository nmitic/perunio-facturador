package signature

import (
	"sync"
	"time"
)

// Cache is a small in-process cert cache keyed by certificate row ID. The
// active cert ID changes on activation, so a new ID naturally bypasses any
// stale entry; the TTL only catches edge cases like manual DB edits.
//
// Expected size is one entry per active company (single-digit / low double-
// digit at our scale), so a plain map with opportunistic eviction is enough.
type Cache struct {
	ttl     time.Duration
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	parsed   *ParsedCertificate
	loadedAt time.Time
}

func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		ttl:     ttl,
		entries: make(map[string]cacheEntry),
	}
}

// GetOrLoad returns the parsed cert for id, calling load on a miss. Concurrent
// callers may race on a miss; the loser's result simply overwrites the winner's
// — both values are equivalent.
func (c *Cache) GetOrLoad(id string, load func() (*ParsedCertificate, error)) (*ParsedCertificate, error) {
	if id == "" {
		return load()
	}
	now := time.Now()

	c.mu.RLock()
	if entry, ok := c.entries[id]; ok && now.Sub(entry.loadedAt) < c.ttl {
		c.mu.RUnlock()
		return entry.parsed, nil
	}
	c.mu.RUnlock()

	parsed, err := load()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[id] = cacheEntry{parsed: parsed, loadedAt: now}
	c.mu.Unlock()
	return parsed, nil
}
