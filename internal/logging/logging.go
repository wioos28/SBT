// Package logging is SBT's structured logger. It writes operational events and
// actively redacts anything that looks like a credential so tokens never reach
// the log file or the console.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Level is a log severity. The values mirror security.Level so the two agree.
type Level int

// Levels.
const (
	Info Level = iota
	Notice
	Warning
	Danger
	Critical
)

func (l Level) String() string {
	switch l {
	case Notice:
		return "NOTICE"
	case Warning:
		return "WARNING"
	case Danger:
		return "DANGER"
	case Critical:
		return "CRITICAL"
	default:
		return "INFO"
	}
}

// Logger writes timestamped, redacted lines.
type Logger struct {
	mu   sync.Mutex
	out  io.Writer
	file *os.File
	min  Level
}

// New returns a logger writing to w.
func New(w io.Writer) *Logger {
	if w == nil {
		w = os.Stderr
	}
	return &Logger{out: w}
}

// OpenFile returns a logger that also appends to ~/.sbt/logs/sbt.log.
func OpenFile() (*Logger, error) {
	dir := filepath.Join(sbtHome(), "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "sbt.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &Logger{out: os.Stderr, file: f}, nil
}

func sbtHome() string {
	if v := os.Getenv("SBT_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".sbt-wioos28")
	}
	return filepath.Join(home, ".sbt-wioos28")
}

// SetMin sets the minimum level that is written.
func (l *Logger) SetMin(level Level) { l.min = level }

// Close closes the log file, if any.
func (l *Logger) Close() {
	if l == nil || l.file == nil {
		return
	}
	_ = l.file.Close()
}

// Log writes one redacted line.
func (l *Logger) Log(level Level, format string, args ...any) {
	if l == nil {
		return
	}
	if level < l.min {
		return
	}
	line := fmt.Sprintf("%s %-8s %s\n", time.Now().Format("2006-01-02T15:04:05"), level.String(), Redact(fmt.Sprintf(format, args...)))
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.out != nil {
		_, _ = io.WriteString(l.out, line)
	}
	if l.file != nil {
		_, _ = io.WriteString(l.file, line)
	}
}

// Info logs at INFO.
func (l *Logger) Info(format string, args ...any) { l.Log(Info, format, args...) }

// Notice logs at NOTICE.
func (l *Logger) Notice(format string, args ...any) { l.Log(Notice, format, args...) }

// Warn logs at WARNING.
func (l *Logger) Warn(format string, args ...any) { l.Log(Warning, format, args...) }

// Danger logs at DANGER.
func (l *Logger) Danger(format string, args ...any) { l.Log(Danger, format, args...) }

// Critical logs at CRITICAL.
func (l *Logger) Critical(format string, args ...any) { l.Log(Critical, format, args...) }

// secretPatterns match credential-shaped substrings anywhere in a message.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(token|secret|password|passwd|apikey|api_key|authorization)([=:]\s*|\s+bearer\s+)([A-Za-z0-9._\-+/=]{6,})`),
	regexp.MustCompile(`(?i)bearer\s+([A-Za-z0-9._\-+/=]{6,})`),
	regexp.MustCompile(`(?i)(hf_|sk-|ghp_)[A-Za-z0-9]{6,}`),
}

// Redact masks credential-shaped substrings in s.
func Redact(s string) string {
	out := s
	for _, re := range secretPatterns {
		out = re.ReplaceAllStringFunc(out, func(m string) string {
			return maskMatch(re, m)
		})
	}
	return out
}

func maskMatch(re *regexp.Regexp, m string) string {
	sub := re.FindStringSubmatch(m)
	if len(sub) == 0 {
		return "********"
	}
	if len(sub) >= 4 {
		// keep the label, mask the value
		return strings.Replace(m, sub[3], "********", 1)
	}
	if len(sub) >= 2 {
		return strings.Replace(m, sub[1], "********", 1)
	}
	return "********"
}
