// Package logging provides structured JSON logging with built-in secret
// redaction so passwords, tokens, cookies, and private keys never reach output.
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Field is a single structured log field.
type Field struct {
	Key   string
	Value any
}

// Logger writes structured JSON records to a destination. It is safe for
// concurrent use. Values are redacted before emission.
type Logger struct {
	mu     sync.Mutex
	w      io.Writer
	level  Level
	fields []Field
}

// Level controls verbosity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "info"
	}
}

// New creates a logger writing to w at the given level.
func New(w io.Writer, level string) *Logger {
	if w == nil {
		w = os.Stdout
	}
	return &Logger{w: w, level: ParseLevel(level)}
}

func (lg *Logger) With(fields ...Field) *Logger {
	merged := make([]Field, 0, len(lg.fields)+len(fields))
	merged = append(merged, lg.fields...)
	merged = append(merged, fields...)
	return &Logger{w: lg.w, level: lg.level, fields: merged}
}

func (lg *Logger) Debug(msg string, fields ...Field) { lg.log(LevelDebug, msg, fields) }
func (lg *Logger) Info(msg string, fields ...Field)  { lg.log(LevelInfo, msg, fields) }
func (lg *Logger) Warn(msg string, fields ...Field)  { lg.log(LevelWarn, msg, fields) }
func (lg *Logger) Error(msg string, fields ...Field) { lg.log(LevelError, msg, fields) }

func (lg *Logger) log(level Level, msg string, fields []Field) {
	if level < lg.level {
		return
	}
	rec := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"level": level.String(),
		"msg":   msg,
	}
	for _, f := range lg.fields {
		rec[f.Key] = redact(f.Key, f.Value)
	}
	for _, f := range fields {
		rec[f.Key] = redact(f.Key, f.Value)
	}
	rec["caller"] = shortCaller(3)
	out, err := json.Marshal(rec)
	if err != nil {
		// Fallback that never contains secret-shaped content.
		fmt.Fprintf(lg.w, `{"level":"error","msg":"log marshal failed"}` + "\n")
		return
	}
	lg.mu.Lock()
	defer lg.mu.Unlock()
	lg.w.Write(out)
	lg.w.Write([]byte{'\n'})
}

func shortCaller(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "?"
	}
	if i := strings.LastIndex(file, "/"); i >= 0 {
		file = file[i+1:]
	}
	return fmt.Sprintf("%s:%d", file, line)
}

// redactSensitiveKeys holds field keys whose values must never be emitted.
var redactSensitiveKeys = map[string]bool{
	"password": true, "passwd": true, "secret": true, "token": true,
	"api_key": true, "apikey": true, "cookie": true, "session": true,
	"private_key": true, "privatekey": true, "passphrase": true,
	"recovery_key": true, "credential": true, "credentials": true,
}

func redact(key string, val any) any {
	lk := strings.ToLower(strings.TrimSpace(key))
	if redactSensitiveKeys[lk] || containsSecretToken(lk) {
		return "[REDACTED]"
	}
	switch v := val.(type) {
	case string:
		return redactString(v)
	case []byte:
		return "[redacted-bytes]"
	default:
		return val
	}
}

func containsSecretToken(s string) bool {
	return strings.Contains(s, "password") ||
		strings.Contains(s, "token") ||
		strings.Contains(s, "secret") ||
		strings.Contains(s, "private") ||
		strings.Contains(s, "credential")
}

// redactString masks values that look like bearer tokens, API keys, or
// connection strings carrying embedded credentials.
func redactString(s string) string {
	lower := strings.ToLower(s)
	// Connection strings with key=value credentials.
	if strings.Contains(lower, "password=") || strings.Contains(lower, "passwd=") ||
		strings.Contains(lower, "secret=") || strings.Contains(lower, "api_key=") {
		return "[REDACTED-connection-string]"
	}
	// URLs with embedded userinfo: scheme://user:pass@host
	if i := strings.Index(lower, "://"); i >= 0 {
		if rest := lower[i+3:]; strings.Contains(rest, "@") {
			return "[REDACTED-url-with-credentials]"
		}
	}
	// Bearer tokens and known token prefixes.
	if strings.HasPrefix(lower, "bearer ") || strings.HasPrefix(lower, "pvetoken") {
		return "[REDACTED-token]"
	}
	return s
}
