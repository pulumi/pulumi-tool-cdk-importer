package proxy

import "sync"

// Capture describes a single intercepted resource that should be written to an
// import file entry.
type Capture struct {
	Type        string
	Name        string
	LogicalName string
	ID          string
}

// CaptureCollector safely aggregates Capture entries from multiple goroutines.
type CaptureCollector struct {
	mu      sync.Mutex
	entries []Capture
	seen    map[string]struct{}
}

// NewCaptureCollector constructs an empty collector.
func NewCaptureCollector() *CaptureCollector {
	return &CaptureCollector{seen: make(map[string]struct{})}
}

// Append records a capture, deduplicating identical entries.
func (c *CaptureCollector) Append(entry Capture) {
	if c == nil {
		return
	}
	key := entry.Type + "|" + entry.Name + "|" + entry.LogicalName + "|" + entry.ID
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.seen[key]; ok {
		return
	}
	c.seen[key] = struct{}{}
	c.entries = append(c.entries, entry)
}

// Results returns a copy of the collected entries.
func (c *CaptureCollector) Results() []Capture {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Capture, len(c.entries))
	copy(out, c.entries)
	return out
}

// Count returns the number of unique captures stored.
func (c *CaptureCollector) Count() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
