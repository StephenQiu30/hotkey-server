package observability

import "sync"

// Counter is a simple thread-safe counter metric.
type Counter struct {
	name  string
	value int64
	mu    sync.Mutex
}

// NewCounter creates a new counter with the given name.
func NewCounter(name string) *Counter {
	return &Counter{name: name}
}

// Name returns the counter's metric name.
func (c *Counter) Name() string {
	return c.name
}

// Inc increments the counter by 1.
func (c *Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value++
}

// Value returns the current counter value.
func (c *Counter) Value() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}
