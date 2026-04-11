package greclient

import (
	"sync"
	"time"
)

// tokenEntry is a cached OAuth2 token with expiry information.
type tokenEntry struct {
	accessToken string
	expiresAt   time.Time
}

// tokenCache is a thread-safe cache of tokens keyed by (companyID, env).
// Tokens live for expires_in seconds (typically 3600); we proactively
// refresh a minute early to avoid racing with SUNAT-side expiry.
type tokenCache struct {
	mu      sync.Mutex
	entries map[string]tokenEntry
}

const tokenRefreshBuffer = 60 * time.Second

func newTokenCache() *tokenCache {
	return &tokenCache{entries: make(map[string]tokenEntry)}
}

// get returns a cached token if present and not near expiry.
func (c *tokenCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if time.Until(entry.expiresAt) <= tokenRefreshBuffer {
		delete(c.entries, key)
		return "", false
	}
	return entry.accessToken, true
}

// put stores a token keyed by (companyID, env). A zero expiresIn is
// treated as 3600 seconds (the SUNAT default).
func (c *tokenCache) put(key, accessToken string, expiresIn int) {
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = tokenEntry{
		accessToken: accessToken,
		expiresAt:   time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
}

// invalidate drops a cached entry — used when SUNAT returns 401 on send.
func (c *tokenCache) invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}
