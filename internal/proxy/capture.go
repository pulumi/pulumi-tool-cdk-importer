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
	total   int
	skipped []SkippedCapture
}

// SkippedCapture holds metadata about resources we could not capture.
type SkippedCapture struct {
	Type        string
	LogicalName string
	Reason      string
}

// CaptureSummary summarizes what happened during capture mode.
type CaptureSummary struct {
	TotalIntercepts int
	UniqueResources int
	Skipped         []SkippedCapture
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
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total++
	key := entry.Type + "|" + entry.Name + "|" + entry.LogicalName + "|" + entry.ID
	if _, ok := c.seen[key]; ok {
		return
	}
	c.seen[key] = struct{}{}
	c.entries = append(c.entries, entry)
}

// Skip records a resource that capture mode could not process.
func (c *CaptureCollector) Skip(skipped SkippedCapture) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.skipped = append(c.skipped, skipped)
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

// Summary produces a snapshot of capture progress for logging.
func (c *CaptureCollector) Summary() CaptureSummary {
	if c == nil {
		return CaptureSummary{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	skipped := make([]SkippedCapture, len(c.skipped))
	copy(skipped, c.skipped)
	return CaptureSummary{
		TotalIntercepts: c.total,
		UniqueResources: len(c.entries),
		Skipped:         skipped,
	}
}
