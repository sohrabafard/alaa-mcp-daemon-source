package logging

import (
	"log/slog"
	"os"
)

type Logger struct {
	Slog   *slog.Logger
	writer *RotateWriter
}

func NewDaemonLogger(path string) (*Logger, error) {
	writer, err := NewRotateWriter(path, 10*1024*1024, 5)
	if err != nil {
		return nil, err
	}
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &Logger{Slog: slog.New(handler), writer: writer}, nil
}

func NewStderrLogger() *Logger {
	return &Logger{Slog: slog.New(slog.NewTextHandler(os.Stderr, nil))}
}

func (l *Logger) Close() error {
	if l == nil || l.writer == nil {
		return nil
	}
	return l.writer.Close()
}
