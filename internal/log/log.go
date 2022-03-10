package log

import (
	"fmt"
	"io"
	"log"
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
	l.Printf("[ERROR] %s", fmt.Sprintf(msg, args...))
}

// Warnf prints a warn log-level message.
func (l Logger) Warnf(msg string, args ...interface{}) {
	l.Printf("[WARN] %s", fmt.Sprintf(msg, args...))
}

// Infof prints an info log-level message.
func (l Logger) Infof(msg string, args ...interface{}) {
	l.Printf("[INFO] %s", fmt.Sprintf(msg, args...))
}
