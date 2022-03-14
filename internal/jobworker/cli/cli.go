// Package cli defines the jobworker CLI.
package cli

import (
	"context"
	"fmt"
	"os"
)

const (
  ecSuccess = iota
)

// TODO: support flags
const (
	serveSub = "serve"
	reexecSub = "reexec"
)

// Run is the entrypoint of the jobworker CLI.
func Run() int {
	if len(os.Args) < 2 {
		return help()
	}

  ctx, cancel := context.WithCancel(context.Background())
  defer cancel()

	switch os.Args[1] {
	case serveSub:
		return runServe(ctx)
	case reexecSub:
		return runReexec(ctx)
	default:
		return help()
	}
}

func help() int {
	fmt.Fprintf(
		os.Stdout,
		"",
	)
  // FIXME: should this be success?
  return ecSuccess
}

