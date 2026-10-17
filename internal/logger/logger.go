package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// RingBuffer stores the most recent log lines in a circular buffer.
type RingBuffer struct {
	mu        sync.RWMutex
	lines     []string
	maxLines  int
	listeners map[int]func(string)
	nextID    int
}

// NewRingBuffer creates a bounded ring buffer for log lines.
func NewRingBuffer(maxLines int) *RingBuffer {
	if maxLines <= 0 {
		maxLines = 1500
	}
	return &RingBuffer{
		lines:     make([]string, 0, maxLines),
		maxLines:  maxLines,
		listeners: make(map[int]func(string)),
	}
}

// Append adds a formatted line to the ring buffer and notifies listeners.
func (rb *RingBuffer) Append(line string) {
	// Truncate excessively long lines (e.g. huge raw dumps) to protect UI rendering performance
	if len(line) > 500 {
		line = line[:500] + " ... [truncado]"
	}

	rb.mu.Lock()
	if len(rb.lines) >= rb.maxLines {
		rb.lines = rb.lines[1:]
	}
	rb.lines = append(rb.lines, line)

	listeners := make([]func(string), 0, len(rb.listeners))
	for _, l := range rb.listeners {
		listeners = append(listeners, l)
	}
	rb.mu.Unlock()

	for _, l := range listeners {
		l(line)
	}
}

// Lines returns a snapshot of all lines currently stored.
func (rb *RingBuffer) Lines() []string {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	res := make([]string, len(rb.lines))
	copy(res, rb.lines)
	return res
}

// Clear clears all stored lines.
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.lines = rb.lines[:0]
}

// RegisterListener registers a callback that will be called whenever a new line arrives.
// Returns an unregister function.
func (rb *RingBuffer) RegisterListener(f func(string)) func() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	id := rb.nextID
	rb.nextID++
	rb.listeners[id] = f

	return func() {
		rb.mu.Lock()
		defer rb.mu.Unlock()
		delete(rb.listeners, id)
	}
}

// ringWriter adapts RingBuffer to io.Writer.
type ringWriter struct {
	rb *RingBuffer
}

func (w *ringWriter) Write(p []byte) (n int, err error) {
	str := string(p)
	// Strip trailing newline if present, as Append will store per line
	if len(str) > 0 && str[len(str)-1] == '\n' {
		str = str[:len(str)-1]
	}
	w.rb.Append(str)
	return len(p), nil
}

var (
	globalBuffer *RingBuffer
	initOnce     sync.Once
)

// InitGlobalLogger initializes global file and ring buffer logging.
func InitGlobalLogger(logFilePath string) *RingBuffer {
	initOnce.Do(func() {
		globalBuffer = NewRingBuffer(1500)

		writers := []io.Writer{os.Stderr, &ringWriter{rb: globalBuffer}}

		if logFilePath != "" {
			f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err == nil {
				writers = append(writers, f)
			} else {
				log.Printf("Warning: could not open log file %s: %v", logFilePath, err)
			}
		}

		multi := io.MultiWriter(writers...)
		log.SetOutput(multi)

		// Start periodic memory stat logger every 60s
		go startMemoryMonitor(globalBuffer)
	})

	return globalBuffer
}

// GetBuffer returns the global ring buffer instance.
func GetBuffer() *RingBuffer {
	if globalBuffer == nil {
		return InitGlobalLogger("")
	}
	return globalBuffer
}

func startMemoryMonitor(rb *RingBuffer) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		allocMB := float64(m.Alloc) / 1024 / 1024
		sysMB := float64(m.Sys) / 1024 / 1024
		goroutines := runtime.NumGoroutine()

		msg := fmt.Sprintf("[Diagnóstico/Memória] Alloc: %.1f MB | Sys: %.1f MB | Goroutines: %d | NumGC: %d",
			allocMB, sysMB, goroutines, m.NumGC)
		log.Println(msg)
	}
}

// waLoggerWrapper implements waLog.Logger backed by our RingBuffer and standard log.
type waLoggerWrapper struct {
	module string
}

// WALogger creates a waLog.Logger that logs to our multi-writer system.
func WALogger(module string) waLog.Logger {
	return &waLoggerWrapper{module: module}
}

func (w *waLoggerWrapper) output(level, msg string, args ...any) {
	formatted := fmt.Sprintf(msg, args...)
	timestamp := time.Now().Format("15:04:05")
	line := fmt.Sprintf("%s [%s/%s] %s", timestamp, w.module, level, formatted)
	log.Println(line)
}

func (w *waLoggerWrapper) Debugf(msg string, args ...any) {
	if os.Getenv("WHATS_GTK_DEBUG") == "1" || os.Getenv("WHATS_GTK_DEBUG") == "true" {
		w.output("DEBUG", msg, args...)
	}
}

func (w *waLoggerWrapper) Infof(msg string, args ...any) {
	w.output("INFO", msg, args...)
}

func (w *waLoggerWrapper) Warnf(msg string, args ...any) {
	w.output("WARN", msg, args...)
}

func (w *waLoggerWrapper) Errorf(msg string, args ...any) {
	w.output("ERROR", msg, args...)
}

func (w *waLoggerWrapper) Sub(subModule string) waLog.Logger {
	return &waLoggerWrapper{module: w.module + "/" + subModule}
}
