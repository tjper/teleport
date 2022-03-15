// Package cli defines the jobworker CLI.
package cli

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/tjper/teleport/internal/jobworker"
)

var (
	key    = flag.String("key", "", "path to private key")
	cert   = flag.String("cert", "", "path to certificate")
	caCert = flag.String("ca_cert", "", "path to CA certificate")
	port   = flag.Int("port", 8080, "port to serve jobworker API")
)

const (
	ecSuccess = iota
	ecCgroupService
	ecJobService
	ecLoadx509
	ecLoadCaCert
	ecBuildCaCert
	ecListen
	ecServe
)

// TODO: support flags
const (
	serveSub = "serve"
)

// Run is the entrypoint of the jobworker CLI.
func Run() int {
	flag.Parse()
	if len(*key) == 0 || len(*cert) == 0 || len(*caCert) == 0 {
		return help()
	}

	if len(os.Args) < 2 {
		return help()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	switch os.Args[1] {
	case serveSub:
		return runServe(ctx)
	case jobworker.Reexec:
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
