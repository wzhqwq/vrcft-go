package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

const diagnosticCapacity = 200
const diagnosticFileBytes int64 = 5 * 1024 * 1024
const diagnosticMessageBytes = 4096

// DiagnosticEntry is a bounded, redacted operational record, never a config
// document or a frame. IDs correlate the retained failure with local JSON logs.
type DiagnosticEntry struct {
	ID        string `json:"id"`
	Time      string `json:"time"`
	Level     string `json:"level"`
	Component string `json:"component"`
	Stage     string `json:"stage"`
	Message   string `json:"message"`
}

type DiagnosticsResponse struct {
	Entries   []DiagnosticEntry `json:"entries"`
	Failure   *DiagnosticEntry  `json:"failure,omitempty"`
	LogPath   string            `json:"logPath"`
	DiskError string            `json:"diskError"`
}

type diagnosticLog struct {
	mu           sync.Mutex
	entries      []DiagnosticEntry
	failure      *DiagnosticEntry
	sequence     uint64
	prefix       string
	path         string
	diskError    string
	dropped      uint64
	queue        chan DiagnosticEntry
	done         chan struct{}
	closed       bool
	ingressStart time.Time
	ingressCount int
}

func newDiagnosticLog() *diagnosticLog {
	return &diagnosticLog{entries: make([]DiagnosticEntry, 0, diagnosticCapacity), prefix: fmt.Sprintf("%x", time.Now().UnixNano())}
}

func (d *diagnosticLog) logger() *slog.Logger { return slog.New(&diagnosticHandler{log: d}) }

// open starts a single disk writer. Memory records precede disk initialization
// so even path/environment failures remain queryable. No I/O runs on frame work.
func (d *diagnosticLog) open(directory string, maxBytes int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.queue != nil {
		return
	}
	d.path = filepath.Join(directory, "application.jsonl")
	if maxBytes <= 0 {
		maxBytes = diagnosticFileBytes
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		d.diskError = redactDiagnostic(fmt.Sprintf("create log directory: %v", err))
		return
	}
	file, err := os.OpenFile(d.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		d.diskError = redactDiagnostic(fmt.Sprintf("open log file: %v", err))
		return
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		d.diskError = redactDiagnostic(err.Error())
		return
	}
	d.queue = make(chan DiagnosticEntry, 256)
	d.done = make(chan struct{})
	for _, entry := range d.entries {
		d.queue <- entry
	}
	go d.runWriter(file, info.Size(), maxBytes, d.queue, d.done)
}

func (d *diagnosticLog) runWriter(file *os.File, size, maxBytes int64, queue <-chan DiagnosticEntry, done chan<- struct{}) {
	defer close(done)
	fail := func(err error) {
		d.mu.Lock()
		d.diskError = redactDiagnostic(fmt.Sprintf("write log file: %v", err))
		d.mu.Unlock()
	}
	defer func() {
		if file != nil {
			if err := file.Close(); err != nil {
				fail(err)
			}
		}
	}()
	for entry := range queue {
		if file == nil {
			continue
		}
		data, err := json.Marshal(entry)
		if err != nil {
			fail(err)
			continue
		}
		data = append(data, '\n')
		if size > 0 && size+int64(len(data)) > maxBytes {
			if err = file.Close(); err != nil {
				file = nil
				fail(err)
				continue
			}
			file = nil
			if err = rotateDiagnosticFiles(d.path); err != nil {
				fail(err)
				continue
			}
			file, err = os.OpenFile(d.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				fail(err)
				continue
			}
			size = 0
		}
		n, err := file.Write(data)
		size += int64(n)
		if err != nil {
			fail(err)
			file.Close()
			file = nil
		}
	}
}

func rotateDiagnosticFiles(path string) error {
	rotated := func(index int) string { return strings.TrimSuffix(path, ".jsonl") + fmt.Sprintf(".%d.jsonl", index) }
	if err := os.Remove(rotated(4)); err != nil && !os.IsNotExist(err) {
		return err
	}
	for i := 3; i >= 0; i-- {
		source := path
		if i > 0 {
			source = rotated(i)
		}
		if err := os.Rename(source, rotated(i+1)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (d *diagnosticLog) write(level slog.Level, component, stage, message string) DiagnosticEntry {
	record := slog.NewRecord(time.Now(), level, message, 0)
	record.AddAttrs(slog.String("component", component), slog.String("stage", stage))
	return d.record(record, nil)
}

func (d *diagnosticLog) record(record slog.Record, attrs []slog.Attr) DiagnosticEntry {
	entry := DiagnosticEntry{Time: record.Time.UTC().Format(time.RFC3339Nano), Level: record.Level.String(), Component: "application", Message: redactDiagnostic(record.Message)}
	apply := func(attr slog.Attr) bool {
		switch attr.Key {
		case "component":
			entry.Component = boundedMessage(redactDiagnostic(attr.Value.String()))
		case "stage":
			entry.Stage = boundedMessage(redactDiagnostic(attr.Value.String()))
		}
		return true
	}
	for _, attr := range attrs {
		apply(attr)
	}
	record.Attrs(apply)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return entry
	}
	d.sequence++
	entry.ID = fmt.Sprintf("%s-%d", d.prefix, d.sequence)
	if len(d.entries) == diagnosticCapacity {
		copy(d.entries, d.entries[1:])
		d.entries[len(d.entries)-1] = entry
	} else {
		d.entries = append(d.entries, entry)
	}
	if d.queue != nil {
		select {
		case d.queue <- entry:
		default:
			d.dropped++
		}
	}
	return entry
}

func (d *diagnosticLog) retainFailure(entry DiagnosticEntry) {
	d.mu.Lock()
	defer d.mu.Unlock()
	copy := entry
	d.failure = &copy
}

func (d *diagnosticLog) snapshot() DiagnosticsResponse {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := DiagnosticsResponse{Entries: append([]DiagnosticEntry{}, d.entries...), LogPath: d.path, DiskError: d.diskError}
	if d.failure != nil {
		copy := *d.failure
		result.Failure = &copy
	}
	if d.dropped > 0 {
		result.DiskError = fmt.Sprintf("%s Disk log queue full: %d records omitted; recent memory logs retained.", result.DiskError, d.dropped)
	}
	return result
}

func (d *diagnosticLog) close() {
	d.mu.Lock()
	if !d.closed {
		d.closed = true
		if d.queue != nil {
			close(d.queue)
		}
	}
	done := d.done
	d.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (d *diagnosticLog) reportFrontend(stage, message string) {
	switch stage {
	case "get_status", "parse_status", "revision", "subscribe":
	default:
		return
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	now := time.Now()
	if now.Sub(d.ingressStart) >= time.Minute {
		d.ingressStart = now
		d.ingressCount = 0
	}
	if d.ingressCount >= 10 {
		d.mu.Unlock()
		return
	}
	d.ingressCount++
	d.mu.Unlock()
	d.write(slog.LevelError, "frontend", stage, message)
}

type diagnosticHandler struct {
	log   *diagnosticLog
	attrs []slog.Attr
	group string
}

func (h *diagnosticHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *diagnosticHandler) Handle(_ context.Context, record slog.Record) error {
	h.log.record(record, h.attrs)
	return nil
}
func (h *diagnosticHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copy := *h
	copy.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &copy
}
func (h *diagnosticHandler) WithGroup(name string) slog.Handler {
	copy := *h
	copy.group = name
	return &copy
}

var diagnosticCredentials = regexp.MustCompile(`(?i)(["']?(?:password|passwd|token|secret|api[_-]?key|authorization|cookie|sessionId)["']?\s*[:=]\s*)(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^\s;,}]+)`)
var diagnosticHeaders = regexp.MustCompile(`(?im)(\b(?:authorization|proxy-authorization|cookie|set-cookie)\s*:\s*)[^\r\n]+`)
var diagnosticBearer = regexp.MustCompile(`(?i)\bBearer\s+[^\s;,]+`)
var diagnosticUserPath = regexp.MustCompile(`(?i)([a-z]:[\\/]Users[\\/]|/home/|/Users/)[^\\/\r\n:;"']+`)
var diagnosticURLCredentials = regexp.MustCompile(`(://)[^\s/@]+:[^\s/@]+@`)

func redactDiagnostic(message string) string {
	// Bound work before regex processing; reserve enough input to redact secrets
	// that straddle the final display boundary.
	if len(message) > 64*1024 {
		message = message[:64*1024]
	}
	message = stringsToValidUTF8(message)
	message = diagnosticHeaders.ReplaceAllString(message, "${1}[redacted]")
	message = diagnosticBearer.ReplaceAllString(message, "Bearer [redacted]")
	message = diagnosticCredentials.ReplaceAllString(message, "${1}[redacted]")
	message = diagnosticUserPath.ReplaceAllString(message, "${1}[user]")
	message = diagnosticURLCredentials.ReplaceAllString(message, "${1}[redacted]@")
	message = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return ' '
		}
		return r
	}, message)
	if len(message) > diagnosticMessageBytes {
		message = strings.ToValidUTF8(message[:diagnosticMessageBytes-3], "") + "…"
	}
	return message
}
