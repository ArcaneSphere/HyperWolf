package router

import (
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log entry.
type LogLevel string

const (
	LogLevelInfo    LogLevel = "INFO"
	LogLevelWarn    LogLevel = "WARN"
	LogLevelError   LogLevel = "ERROR"
	LogLevelSuccess LogLevel = "SUCCESS"
)

// LogEntry represents a single log event with timestamp, level, and message.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     LogLevel  `json:"level"`
	Message   string    `json:"message"`
}

// LogService is a thread-safe in-memory log store that captures
// important application events for display in the terminal log page.
type LogService struct {
	mu      sync.RWMutex
	entries []LogEntry
	subs    []chan []LogEntry
}

// NewLogService creates a new LogService instance.
func NewLogService() *LogService {
	return &LogService{
		entries: make([]LogEntry, 0, 200),
	}
}

// Add appends a new log entry and notifies all subscribers.
func (ls *LogService) Add(level LogLevel, message string) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}

	ls.mu.Lock()
	ls.entries = append(ls.entries, entry)
	// Keep only the last 2000 entries to avoid unbounded memory growth.
	if len(ls.entries) > 2000 {
		ls.entries = ls.entries[len(ls.entries)-2000:]
	}
	subs := make([]chan []LogEntry, len(ls.subs))
	copy(subs, ls.subs)
	ls.mu.Unlock()

	// Notify subscribers (non-blocking)
	for _, ch := range subs {
		select {
		case ch <- []LogEntry{entry}:
		default:
			// Subscriber is behind, skip
		}
	}
}

// AddMessage classifies a raw, free-form log line by severity and stores it.
// This is the entry point used by the standard-logger bridge so that every
// log.Printf / log.Println in the app also lands on the Terminal Logs page.
func (ls *LogService) AddMessage(message string) {
	ls.Add(ClassifyLevel(message), message)
}

// ClassifyLevel infers a LogLevel from free-form log text using simple
// keyword heuristics. Unknown lines are treated as INFO.
func ClassifyLevel(message string) LogLevel {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "panic"),
		strings.Contains(lower, "fatal"),
		strings.Contains(lower, "error"),
		strings.Contains(lower, "failed"),
		strings.Contains(lower, "unreachable"),
		strings.Contains(lower, "exited"):
		return LogLevelError
	case strings.Contains(lower, "warn"),
		strings.Contains(lower, "warning"),
		strings.Contains(lower, "unable"),
		strings.Contains(lower, "cannot"):
		return LogLevelWarn
	case strings.Contains(lower, "connected"),
		strings.Contains(lower, "synced"),
		strings.Contains(lower, "complete"),
		strings.Contains(lower, "ready"),
		strings.Contains(lower, "success"),
		strings.Contains(lower, "started"),
		strings.Contains(lower, "listening"):
		return LogLevelSuccess
	default:
		return LogLevelInfo
	}
}

// GetEntries returns all stored log entries, optionally filtered by level.
func (ls *LogService) GetEntries(since time.Time, level string) []LogEntry {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	var result []LogEntry
	for _, entry := range ls.entries {
		if !entry.Timestamp.After(since) && !entry.Timestamp.Equal(since) {
			continue
		}
		if level != "" && string(entry.Level) != level {
			continue
		}
		result = append(result, entry)
	}
	return result
}

// Subscribe returns a channel that receives new log entries as they're added
// and an unregister function that MUST be called when the subscriber is done.
// The returned channel is closed when unregister is called, causing any
// range-loop over it to exit cleanly.
func (ls *LogService) Subscribe() (<-chan []LogEntry, func()) {
	ch := make(chan []LogEntry, 20)
	ls.mu.Lock()
	ls.subs = append(ls.subs, ch)
	ls.mu.Unlock()

	var once sync.Once
	unregister := func() {
		once.Do(func() {
			ls.mu.Lock()
			for i, sub := range ls.subs {
				if sub == ch {
					ls.subs = append(ls.subs[:i], ls.subs[i+1:]...)
					break
				}
			}
			ls.mu.Unlock()
			close(ch)
		})
	}
	return ch, unregister
}

// GetRecentEntries returns the most recent N entries.
func (ls *LogService) GetRecentEntries(n int) []LogEntry {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	if n > len(ls.entries) {
		n = len(ls.entries)
	}
	if n == 0 {
		return []LogEntry{}
	}

	result := make([]LogEntry, n)
	copy(result, ls.entries[len(ls.entries)-n:])
	return result
}

// Clear removes all log entries.
func (ls *LogService) Clear() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.entries = ls.entries[:0]
}
