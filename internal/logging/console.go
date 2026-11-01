package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// consoleHandler keeps presentation separate from the structured file records.
// Derived handlers share a lock but never mutate each other's attributes.
type consoleHandler struct {
	output io.Writer
	level  slog.Leveler
	mu     *sync.Mutex
	attrs  []slog.Attr
	group  string
}

func (h *consoleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *consoleHandler) Handle(_ context.Context, record slog.Record) error {
	var line strings.Builder
	fmt.Fprintf(&line, "%s %s", record.Level, consoleValue(record.Message, false))
	appendAttr := func(attr slog.Attr) {
		switch attr.Key {
		case "workspace", "pid", "command", "vm":
			return
		case "duration_ms":
			if attr.Value.Kind() == slog.KindInt64 {
				duration := (time.Duration(attr.Value.Int64()) * time.Millisecond).Round(time.Millisecond)
				fmt.Fprintf(&line, " (%s)", duration)
				return
			}
		}
		fmt.Fprintf(&line, " %s=%s", consoleValue(attr.Key, true), consoleValue(attr.Value.String(), true))
	}
	for _, attr := range h.attrs {
		appendAttr(attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		consoleAttrs(h.group, attr, appendAttr)
		return true
	})
	line.WriteByte('\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.output, line.String())
	return err
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append([]slog.Attr(nil), h.attrs...)
	for _, attr := range attrs {
		consoleAttrs(h.group, attr, func(attr slog.Attr) { clone.attrs = append(clone.attrs, attr) })
	}
	return &clone
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.group += name + "."
	return &clone
}

// Resolve values when bound, flatten groups, and ignore empty attributes/groups,
// following slog's handler contract.
func consoleAttrs(prefix string, attr slog.Attr, emit func(slog.Attr)) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		if attr.Key != "" {
			prefix += attr.Key + "."
		}
		for _, child := range attr.Value.Group() {
			consoleAttrs(prefix, child, emit)
		}
		return
	}
	attr.Key = prefix + attr.Key
	emit(attr)
}

func consoleValue(value string, quoteSpaces bool) string {
	if value == "" || strings.ContainsAny(value, "\"\\=") || strings.ContainsFunc(value, func(r rune) bool {
		return unicode.IsControl(r) || quoteSpaces && unicode.IsSpace(r)
	}) {
		return strconv.Quote(value)
	}
	return value
}
