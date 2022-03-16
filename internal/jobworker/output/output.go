// Package output provides utilities for interacting with jobworker log output.
package output

import (
	"fmt"
	"path"
)

const (
	// Root is the default jobworker log output root directory.
	Root = "/var/log/jobworker"
	// FileMode is the default FileMode for log output resources.
	FileMode = 0644
)

// File returns the standard jobworker log file location based on the passed
// id.
func File(id fmt.Stringer) string {
	return path.Join(Root, fmt.Sprintf("%s.log", id.String()))
}
