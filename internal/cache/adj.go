package cache

import (
	"container/list"
	"sync"

	"github.com/atharv3903/graphion/internal/model"
)

// defaultAdjCapacity is the default number of adjacency entries the cache will hold.
// Change as needed or use NewAdjCacheWithCap for a custom size.
const defaultAdjCapacity = 2048

type adjEntry struct {
	key int64
	val []model.Edge
}

// AdjCache is a bounded LRU cache for adjacency lists.
// It's safe for concurrent use.
type AdjCache struct {
	mu        sync.Mutex
	m         map[int64]*list.Element
	ll        *list.List
	capacity  int
	// stats
	puts      int
	gets      int
	hits      int
	evictions int
}

// NewAdjCache returns an LRU adj cache with the default capacity.
func NewAdjCache() *AdjCache {
	return NewAdjCacheWithCap(defaultAdjCapacity)
}

// NewAdjCacheWithCap returns an LRU adj cache with the provided capacity.
// capacity must be > 0.
func NewAdjCacheWithCap(capacity int) *AdjCache {
	if capacity <= 0 {
		capacity = defaultAdjCapacity
	}
	return &AdjCache{
		m:        make(map[int64]*list.Element, capacity),
		ll:       list.New(),
		capacity: capacity,
	}
}

// Get returns the adjacency list for key, and true if it was found.
// It updates LRU position on hit.
func (c *AdjCache) Get(key int64) ([]model.Edge, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.gets++
	if el, ok := c.m[key]; ok {
		c.hits++
		c.ll.MoveToFront(el)
		return el.Value.(adjEntry).val, true
	}
	return nil, false
}

// Put inserts the adjacency list into the cache. If insertion causes the cache to exceed
// capacity, the least-recently-used entry is evicted.
func (c *AdjCache) Put(key int64, v []model.Edge) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// if present, update value and move to front
	if el, ok := c.m[key]; ok {
		el.Value = adjEntry{key: key, val: v}
		c.ll.MoveToFront(el)
		// count as a logical put (replacement)
		c.puts++
		return
	}

	// new entry
	el := c.ll.PushFront(adjEntry{key: key, val: v})
	c.m[key] = el
	c.puts++

	// evict if over capacity
	if c.ll.Len() > c.capacity {
		tail := c.ll.Back()
		if tail != nil {
			ae := tail.Value.(adjEntry)
			delete(c.m, ae.key)
			c.ll.Remove(tail)
			c.evictions++
		}
	}
}

// Invalidate removes a single adjacency entry (if present).
func (c *AdjCache) Invalidate(key int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.m[key]; ok {
		delete(c.m, key)
		c.ll.Remove(el)
		// We don't change puts/hits here; evictions is only for LRU-driven evictions.
	}
}

// Clear fully resets the cache and stats.
func (c *AdjCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = make(map[int64]*list.Element, c.capacity)
	c.ll.Init()
	c.puts = 0
	c.gets = 0
	c.hits = 0
	c.evictions = 0
}

// Stats returns (gets, hits, puts, evictions) — all snapshot under lock.
func (c *AdjCache) Stats() (gets, hits, puts, evictions int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gets, c.hits, c.puts, c.evictions
}
