package queqiao

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/sagernet/sing-box/log"
)

type slogHandler struct {
	logger log.ContextLogger
	attrs  []slog.Attr
	group  string
}

func newLogger(logger log.ContextLogger) *slog.Logger           { return slog.New(&slogHandler{logger: logger}) }
func (h *slogHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *slogHandler) Handle(ctx context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	write := func(a slog.Attr) { fmt.Fprintf(&b, " %s%s=%v", h.group, a.Key, a.Value.Resolve()) }
	for _, a := range h.attrs {
		write(a)
	}
	r.Attrs(func(a slog.Attr) bool { write(a); return true })
	switch {
	case r.Level >= slog.LevelError:
		h.logger.ErrorContext(ctx, b.String())
	case r.Level >= slog.LevelWarn:
		h.logger.WarnContext(ctx, b.String())
	case r.Level >= slog.LevelInfo:
		h.logger.InfoContext(ctx, b.String())
	default:
		h.logger.DebugContext(ctx, b.String())
	}
	return nil
}
func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &slogHandler{h.logger, append(append([]slog.Attr(nil), h.attrs...), attrs...), h.group}
}
func (h *slogHandler) WithGroup(name string) slog.Handler {
	return &slogHandler{h.logger, h.attrs, h.group + name + "."}
}
