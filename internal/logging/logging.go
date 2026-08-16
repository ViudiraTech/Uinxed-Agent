package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var secretRE = regexp.MustCompile(`(?i)(sk-[a-z0-9_\-]{8,}|bearer\s+[a-z0-9._\-]{8,}|api[_-]?key["'=:\s]+[a-z0-9._\-]{8,})`)

type redactHandler struct{ next slog.Handler }

func (h redactHandler) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }
func (h redactHandler) Handle(ctx context.Context, r slog.Record) error {
	r.Message = redact(r.Message)
	var attrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool { attrs = append(attrs, redactAttr(a)); return true })
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	nr.AddAttrs(attrs...)
	return h.next.Handle(ctx, nr)
}
func (h redactHandler) WithAttrs(a []slog.Attr) slog.Handler {
	b := make([]slog.Attr, len(a))
	for i := range a {
		b[i] = redactAttr(a[i])
	}
	return redactHandler{h.next.WithAttrs(b)}
}
func (h redactHandler) WithGroup(n string) slog.Handler { return redactHandler{h.next.WithGroup(n)} }
func redactAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindString {
		a.Value = slog.StringValue(redact(a.Value.String()))
	}
	return a
}
func redact(s string) string {
	return secretRE.ReplaceAllStringFunc(s, func(v string) string {
		if strings.HasPrefix(strings.ToLower(v), "bearer ") {
			return "Bearer ****"
		}
		return "****"
	})
}

func Open(stateDir string, debug bool) (*slog.Logger, io.Closer, error) {
	dir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	path := filepath.Join(dir, "ux-agent-"+time.Now().Format("20060102")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	h := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})
	return slog.New(redactHandler{h}), f, nil
}
