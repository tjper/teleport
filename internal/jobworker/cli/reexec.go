package cli

import (
	"context"

	"github.com/tjper/teleport/internal/jobworker/reexec"
)

// runReexec is called as a child process. This logic will read Job data from
// the parent and execute an arbitrary command specific to the Job.
func runReexec(ctx context.Context) int {
	exitCode, err := reexec.Exec(ctx)
	if err != nil {
		logger.Errorf("reexec; error: %s", err)
	}
	return exitCode
}
