// Package diagnostics keeps a small rolling log for the UI and a log file for
// troubleshooting. It never records PINs or key material.
package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const keepLines = 300

// Log is a concurrency-safe rolling log.
type Log struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	maxBytes int64
	lines    []string
	closed   bool
}

// New opens the log at path, creating its directory. A file that cannot open
// still yields a usable in-memory log.
func New(path string) *Log {
	l := &Log{maxBytes: 5 << 20}
	if path == "" {
		path = DefaultPath()
	}
	l.path = path
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
		l.file, _ = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	}
	return l
}

// DefaultPath is %APPDATA%\saundo\saundo.log.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "saundo", "saundo.log")
}

// Printf appends one timestamped line. After Close it is a no-op, so a
// goroutine that logs late during shutdown cannot write a closed file.
func (l *Log) Printf(format string, args ...any) {
	if l == nil {
		return
	}
	line := time.Now().Format("15:04:05.000 ") + fmt.Sprintf(format, args...)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.lines = append(l.lines, line)
	if len(l.lines) > keepLines {
		l.lines = l.lines[len(l.lines)-keepLines:]
	}
	if l.file != nil {
		_, _ = l.file.WriteString(time.Now().Format("2006-01-02 ") + line + "\n")
		l.rotateLocked()
	}
}

// rotateLocked moves an oversized log to path+".1" and reopens a fresh one.
func (l *Log) rotateLocked() {
	info, err := l.file.Stat()
	if err != nil || info.Size() < l.maxBytes {
		return
	}
	_ = l.file.Close()
	_ = os.Remove(l.path + ".1")
	_ = os.Rename(l.path, l.path+".1")
	l.file, _ = os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

// Lines returns a copy of the recent lines.
func (l *Log) Lines() []string {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

// Close closes the log file.
func (l *Log) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	l.closed = true
}
