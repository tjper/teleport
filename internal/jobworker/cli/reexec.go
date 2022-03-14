package cli

import (
	"context"
	"os"

	"github.com/tjper/teleport/internal/jobworker/reexec"
	"github.com/tjper/teleport/internal/log"
)

var logger = log.New(os.Stdout, "cli")

func runReexec(ctx context.Context) int {
	exitCode, err := reexec.Exec(ctx)
	if err != nil {
		logger.Errorf("reexec; error: %s", err)
	}
	return exitCode
}
