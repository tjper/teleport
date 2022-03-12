package errors

import "fmt"

// TODO: add stack trace to wrap

// Wrap returns a new error wrapping the passed error. If the passed error is
// nil, nil is returned.
func Wrap(err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%w", err)
}
