package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// New returns a slog.Logger with a friendly, single-line format.
// Verbosity > 0 enables debug-level logs; otherwise only info-level logs emit.
func New(w io.Writer, debug bool, attrs ...any) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	level := slog.LevelInfo
	if debug {
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
	prefix   string
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
		if h.prefix != "" {
			a.Key = h.prefix + a.Key
		}
		attrs = append(attrs, a)
		return true
	})
	if len(attrs) > 0 {
		b.WriteString(" ")
		for idx, a := range attrs {
			if idx > 0 {
				b.WriteString(" ")
			}
			fmt.Fprintf(&b, "%s=%s", a.Key, formatValue(a.Value))
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String()+"\n")
	return err
}

func formatValue(v slog.Value) string {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return strconv.Quote(v.String())
	case slog.KindInt64:
		return strconv.FormatInt(v.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(v.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(v.Float64(), 'g', -1, 64)
	case slog.KindBool:
		return strconv.FormatBool(v.Bool())
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339Nano)
	case slog.KindAny:
		return formatAny(v.Any())
	case slog.KindGroup:
		return fmt.Sprint(v.Group())
	default:
		return fmt.Sprint(v.Any())
	}
}

func formatAny(val any) string {
	switch v := val.(type) {
	case string:
		return strconv.Quote(v)
	case fmt.Stringer:
		return strconv.Quote(v.String())
	case error:
		return strconv.Quote(v.Error())
	case time.Time:
		return v.Format(time.RFC3339Nano)
	case time.Duration:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func (h *friendlyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	static := make([]any, 0, len(h.static)+len(attrs)*2)
	static = append(static, h.static...)
	for _, a := range attrs {
		key := a.Key
		if h.prefix != "" {
			key = h.prefix + key
		}
		static = append(static, key, a.Value.Any())
	}
	return &friendlyHandler{
		minLevel: h.minLevel,
		w:        h.w,
		static:   static,
		prefix:   h.prefix,
	}
}

func (h *friendlyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &friendlyHandler{
		minLevel: h.minLevel,
		w:        h.w,
		static:   h.static,
		prefix:   h.prefix + name + ".",
	}
}
