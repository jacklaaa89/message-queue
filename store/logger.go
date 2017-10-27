package store

import (
	"fmt"
	"io"
	"os"
	"time"
)

const (
	// LevelInfo logger level for information.
	LevelInfo = "info"
	// LevelWarn logger level for a warning.
	LevelWarn = "warn"
	// LevelFatal logger level for a fatal error.
	LevelFatal = "fatal"
)

const template = `%v - %v - `

// Logger an instance of a logger.
type Logger interface {
	Logf(level, format string, arguments ...interface{}) (int, error)
}

// writeLogger writes to a write closer instance.
type writerLogger struct {
	writer io.Writer
}

// Log logs to the writers.
func (w *writerLogger) Logf(level, format string, arguments ...interface{}) (int, error) {
	args := make([]interface{}, len(arguments)+2)
	args[0], args[1] = level, time.Now().UTC().Format(time.RFC3339)
	copy(args[2:], arguments)
	return fmt.Fprintf(w.writer, template+format+"\n", args...)
}

// newLogger initialises a new logger.
func newLogger() Logger {
	return &writerLogger{
		// MultiWriter allows us to write to many writers at once.
		writer: io.MultiWriter(os.Stdout /*, a file handler etc*/),
	}
}
