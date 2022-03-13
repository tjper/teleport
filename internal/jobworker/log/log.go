// Package log provides utilities for interacting with jobworker log output.
package log

import (
	"fmt"
)

// File returns the standard jobworker log file location based on the passed
// id.
func File(id fmt.Stringer) string {
	return fmt.Sprintf("/var/log/jobworker/%s", id.String())
}
