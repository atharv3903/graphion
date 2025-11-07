package cache

import (
	"sync"
	"github.com/atharv3903/graphion/internal/model"
)

// type AdjCache struct {
// 	mu sync.RWMutex
// 	m  map[int64][]model.Edge
// }

// func NewAdjCache() *AdjCache {
// 	return &AdjCache{m: make(map[int64][]model.Edge)}
// }

// func (c *AdjCache) Get(key int64) ([]model.Edge, bool) {
// 	c.mu.RLock()
// 	v, ok := c.m[key]
// 	c.mu.RUnlock()
// 	return v, ok
// }

// func (c *AdjCache) Put(key int64, v []model.Edge) {
// 	c.mu.Lock()
// 	c.m[key] = v
// 	c.mu.Unlock()
// }

// func (c *AdjCache) Invalidate(key int64) {
// 	c.mu.Lock()
// 	delete(c.m, key)
// 	c.mu.Unlock()
// }



type AdjCache struct {
    mu   sync.RWMutex
    m    map[int64][]model.Edge
    puts int
    gets int
    hits int
}

func NewAdjCache() *AdjCache {
    return &AdjCache{m: make(map[int64][]model.Edge)}
}

func (c *AdjCache) Get(key int64) ([]model.Edge, bool) {
    c.mu.RLock()
    v, ok := c.m[key]
    c.gets++
    if ok {
        c.hits++
    }
    c.mu.RUnlock()
    return v, ok
}

func (c *AdjCache) Put(key int64, v []model.Edge) {
    c.mu.Lock()
    c.m[key] = v
    c.puts++
    c.mu.Unlock()
}

func (c *AdjCache) Invalidate(key int64) {
    c.mu.Lock()
    delete(c.m, key)
    c.mu.Unlock()
}

func (c *AdjCache) Stats() (gets, hits, puts int) {
    c.mu.RLock()
    gets, hits, puts = c.gets, c.hits, c.puts
    c.mu.RUnlock()
    return
}


