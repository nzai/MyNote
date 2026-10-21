package logging

import (
	"io"
	"log/slog"
	"maps"
	"time"
)

const (
	DefaultMaxBytes int64 = 10 * 1024 * 1024
	DefaultMaxFiles       = 5
	DefaultPrefix         = "clipnote"
)

type Options struct {
	Dir         string
	Prefix      string
	MaxBytes    int64
	MaxFiles    int
	Level       slog.Level
	DebugScopes []string
	Now         func() time.Time
}

type Factory struct {
	writer       io.Writer
	defaultLevel slog.Level
	debug        map[string]bool
	closer       io.Closer
}

func New(options Options) (*Factory, error) {
	if options.Prefix == "" {
		options.Prefix = DefaultPrefix
	}
	if options.MaxBytes == 0 {
		options.MaxBytes = DefaultMaxBytes
	}
	if options.MaxFiles == 0 {
		options.MaxFiles = DefaultMaxFiles
	}

	writer, err := NewRotatingWriter(
		options.Dir,
		options.Prefix,
		options.MaxBytes,
		options.MaxFiles,
		options.Now,
	)
	if err != nil {
		return nil, err
	}
	debug := make(map[string]bool, len(options.DebugScopes))
	for _, scope := range options.DebugScopes {
		debug[scope] = true
	}

	factory := NewWithWriter(writer, options.Level, debug)
	factory.closer = writer
	return factory, nil
}

func NewWithWriter(w io.Writer, level slog.Level, scopes map[string]bool) *Factory {
	return &Factory{
		writer:       w,
		defaultLevel: level,
		debug:        maps.Clone(scopes),
	}
}

func (f *Factory) Scope(scope string) *slog.Logger {
	level := f.defaultLevel
	if f.debug[scope] {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(f.writer, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				return slog.String("time", attr.Value.Time().Format(time.RFC3339Nano))
			}
			return attr
		},
	})
	return slog.New(handler).With("scope", scope)
}

func (f *Factory) Close() error {
	if f.closer == nil {
		return nil
	}
	return f.closer.Close()
}
