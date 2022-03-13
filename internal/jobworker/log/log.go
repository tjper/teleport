// TODO: document
package log

import (
	"fmt"
)

func File(id fmt.Stringer) string {
	return fmt.Sprintf("/var/log/jobworker/%s", id.String())
}
