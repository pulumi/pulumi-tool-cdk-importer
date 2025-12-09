package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// New returns a slog.Logger with a friendly, single-line format.
// Verbosity > 0 enables debug-level logs; otherwise only info-level logs emit.
func New(w io.Writer, verbose int, attrs ...any) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	level := slog.LevelInfo
	if verbose > 0 {
		level = slog.LevelDebug
	}
	handler := &friendlyHandler{
		minLevel: level,
		w:        w,
		static:   attrs,
	}
	return slog.New(handler)
}

// friendlyHandler emits concise lines like:
// [INFO] Importing stack... stack=my-stack
type friendlyHandler struct {
	mu       sync.Mutex
	minLevel slog.Level
	w        io.Writer
	static   []any
}

func (h *friendlyHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= h.minLevel
}

func (h *friendlyHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	level := r.Level.String()
	level = strings.ToUpper(level)
	if r.Time.IsZero() {
		r.Time = time.Now()
	}
	fmt.Fprintf(&b, "[%s] %s", level, r.Message)

	attrs := make([]slog.Attr, 0, len(h.static)+int(r.NumAttrs()))
	for i := 0; i+1 < len(h.static); i += 2 {
		attrs = append(attrs, slog.Any(fmt.Sprint(h.static[i]), h.static[i+1]))
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	if len(attrs) > 0 {
		b.WriteString(" ")
		for idx, a := range attrs {
			if idx > 0 {
				b.WriteString(" ")
			}
			fmt.Fprintf(&b, "%s=%v", a.Key, a.Value.Any())
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String()+"\n")
	return err
}

func (h *friendlyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	static := make([]any, 0, len(h.static)+len(attrs)*2)
	static = append(static, h.static...)
	for _, a := range attrs {
		static = append(static, a.Key, a.Value.Any())
	}
	return &friendlyHandler{
		minLevel: h.minLevel,
		w:        h.w,
		static:   static,
	}
}

func (h *friendlyHandler) WithGroup(string) slog.Handler {
	// Grouping is ignored for the friendly handler; keep attrs flat.
	return h
}
