package activity

import "sync"

// memo remembers what a locator learned, so the sidebar's once-a-second refresh looks it up instead of scanning the
// disk again. The zero value is ready to use.
type memo[V any] struct {
	mu sync.Mutex
	m  map[string]V
}

func (c *memo[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[key]
	return v, ok
}

func (c *memo[V]) put(key string, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]V{}
	}
	c.m[key] = v
}

func (c *memo[V]) forget(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, key)
}
