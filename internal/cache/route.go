package cache

import "sync"

type RouteKey struct{ Src, Dst int64; Algo string; Epoch uint64 }

type RouteCache struct {
	mu    sync.RWMutex
	epoch uint64
	m     map[RouteKey][]int64
}

func NewRouteCache() *RouteCache {
	return &RouteCache{m: make(map[RouteKey][]int64)}
}

func (c *RouteCache) Get(k RouteKey) ([]int64, bool) {
	c.mu.RLock()
	v, ok := c.m[k]
	c.mu.RUnlock()
	return v, ok
}

func (c *RouteCache) Put(k RouteKey, p []int64) {
	c.mu.Lock()
	c.m[k] = p
	c.mu.Unlock()
}

func (c *RouteCache) Epoch() uint64 {
	c.mu.RLock()
	e := c.epoch
	c.mu.RUnlock()
	return e
}

func (c *RouteCache) BumpEpoch() {
	c.mu.Lock()
	c.epoch++
	c.mu.Unlock()
}
