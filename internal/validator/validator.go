// Package validator provides utility types and functions for validating
// input.
package validator

import (
	"errors"
	"fmt"
)

// ErrInvalidInput indicates a input validation check failed.
var ErrInvalidInput = errors.New("invalid input")

// NewErrInvalidInput creates a new error wrapping ErrInvalidInput.
func NewErrInvalidInput(msg string) error {
	return fmt.Errorf("%w; msg: %s", ErrInvalidInput, msg)
}

// New creates a Validator instance.
func New() *Validator {
	return &Validator{}
}

// Validator provides a set of methods to ensure arbitrary conditions are true.
// In the event the one condition is false, Validator records the failing
// condition and does not proceed with further checks.
type Validator struct {
	err error
}

// AssertFunc checks that fn returns true, if not msg is used to construct an
// error to be returned by Validator.Err().
func (v *Validator) AssertFunc(fn func() bool, msg string) {
	if v.err != nil {
		return
	}
	if !fn() {
		v.err = NewErrInvalidInput(msg)
	}
}

// Assert checks that condition is true, if not msg is used to construct an
// error to be returned by Validator.Err().
func (v *Validator) Assert(condition bool, msg string) {
	if v.err != nil {
		return
	}
	if !condition {
		v.err = NewErrInvalidInput(msg)
	}
}

// Err returns an error that was encountered during the Validators checks.
func (v Validator) Err() error {
	return v.err
}

// Format provides consistent invalid input messaging.
func Format(msg string) string {
	return fmt.Sprintf("invalid input; %s", msg)
}
