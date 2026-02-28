package main

import (
	"sync"
	"time"
)

type cacheEntry struct {
	markdown string
	time     time.Time
}

type URLCache struct {
	mu   sync.RWMutex
	ttl  time.Duration
	data map[string]cacheEntry
}

func NewURLCache(ttl time.Duration) *URLCache {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	c := &URLCache{
		ttl:  ttl,
		data: make(map[string]cacheEntry),
	}
	go c.cleanupLoop()
	return c
}

func (c *URLCache) Get(key string) (string, bool) {
	c.mu.RLock()
	entry, ok := c.data[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	// Check TTL without deleting — let cleanupLoop handle removal.
	// Deleting here after RLock→Lock upgrade races with concurrent Set():
	// goroutine A finds expired entry, releases RLock;
	// goroutine B calls Set() with fresh value;
	// goroutine A acquires Lock and deletes B's fresh entry.
	if time.Since(entry.time) > c.ttl {
		return "", false
	}
	return entry.markdown, true
}

func (c *URLCache) Set(key, markdown string) {
	c.mu.Lock()
	c.data[key] = cacheEntry{markdown: markdown, time: time.Now()}
	c.mu.Unlock()
}

func (c *URLCache) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for k, v := range c.data {
			if now.Sub(v.time) > c.ttl {
				delete(c.data, k)
			}
		}
		c.mu.Unlock()
	}
}
