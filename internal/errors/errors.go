package errors

import "fmt"

// Wrapf returns a new error wrapping the passed error. The returned error is
// annotated with the passed msg and args. If the passed error is nil, nil is
// returned.
func Wrapf(err error, msg string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("error: %w, msg: %s", err, fmt.Sprintf(msg, args...))
}
