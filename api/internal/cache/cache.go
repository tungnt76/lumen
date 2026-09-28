// Package cache is a small in-memory TTL cache with request coalescing.
package cache

import (
	"sync"
	"time"
)

type entry struct {
	val     []byte
	expires time.Time
}

type call struct {
	wg  sync.WaitGroup
	val []byte
	err error
}

type Cache struct {
	mu       sync.Mutex
	items    map[string]entry
	inflight map[string]*call
	max      int
}

// New creates a cache holding at most max entries.
func New(max int) *Cache {
	c := &Cache{items: map[string]entry{}, inflight: map[string]*call{}, max: max}
	go c.janitor()
	return c
}

func (c *Cache) janitor() {
	t := time.NewTicker(5 * time.Minute)
	for range t.C {
		now := time.Now()
		c.mu.Lock()
		for k, e := range c.items {
			if now.After(e.expires) {
				delete(c.items, k)
			}
		}
		c.mu.Unlock()
	}
}

// GetOrLoad returns the cached value for key, or runs load once (even with
// concurrent callers) and caches the result for ttl.
func (c *Cache) GetOrLoad(key string, ttl time.Duration, load func() ([]byte, error)) ([]byte, error) {
	c.mu.Lock()
	if e, ok := c.items[key]; ok && time.Now().Before(e.expires) {
		c.mu.Unlock()
		return e.val, nil
	}
	if cl, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		cl.wg.Wait()
		return cl.val, cl.err
	}
	cl := &call{}
	cl.wg.Add(1)
	c.inflight[key] = cl
	c.mu.Unlock()

	cl.val, cl.err = load()
	cl.wg.Done()

	c.mu.Lock()
	delete(c.inflight, key)
	if cl.err == nil {
		if len(c.items) >= c.max {
			for k := range c.items { // evict an arbitrary entry
				delete(c.items, k)
				break
			}
		}
		c.items[key] = entry{val: cl.val, expires: time.Now().Add(ttl)}
	}
	c.mu.Unlock()
	return cl.val, cl.err
}
