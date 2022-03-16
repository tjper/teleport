package log

import (
	"fmt"
	"io"
	"log"
	"runtime"
	"strings"
)

// New creates a Logger instance.
func New(w io.Writer, prefix string) *Logger {
	return &Logger{
		log.New(
			w,
			prefix,
			log.Ldate|log.Ltime|log.Lmicroseconds|log.LUTC|log.Lmsgprefix,
		),
	}
}

// Logger represents a logging object that writes output to an io.Writer. Each
// logging operation makes a single call to the Writer's Write method. Logger
// is thread-safe; it guarantees to serialize access to the Writer.
type Logger struct {
	*log.Logger
}

// Errorf prints an error log-level message.
func (l Logger) Errorf(msg string, args ...interface{}) {
	file, line := caller(2)
	l.Printf("[ERROR] %s:%d --- %s", file, line, fmt.Sprintf(msg, args...))
}

// Warnf prints a warn log-level message.
func (l Logger) Warnf(msg string, args ...interface{}) {
	file, line := caller(2)
	l.Printf("[WARN] %s:%d --- %s", file, line, fmt.Sprintf(msg, args...))
}

// Infof prints an info log-level message.
func (l Logger) Infof(msg string, args ...interface{}) {
	file, line := caller(2)
	l.Printf("[INFO] %s:%d --- %s", file, line, fmt.Sprintf(msg, args...))
}

func caller(depth int) (string, int) {
	_, file, line, ok := runtime.Caller(depth)
	parts := strings.Split(file, "/")

	// shorten file if it consists of more than 3 parts
	if len(parts) > 3 {
		file = strings.Join(parts[len(parts)-3:], "/")
	}
	if !ok {
		file = "???"
		line = 0
	}
	return file, line
}
