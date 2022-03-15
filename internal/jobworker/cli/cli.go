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
	key    = flag.String("key", "", "path to server private key")
	cert   = flag.String("cert", "", "path to server certificate")
	caCert = flag.String("ca", "", "path to CA certificate")
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
		`
Jobworker launches a grpc API that allows arbitrary commands to be started, 
stopped, retrieved, and streamed.

Usage:
  jobworker [global flags] command

Available Commands:
  serve       serve jobworker API
  reexec      create grandchild process to execute arbitrary command passed 
              from serve process (should not be called)

Global Flags:
  -port      port to serve jobworker API
  -cert      server x509 certificate
  -key       server private key
  -ca        certificate authority cert
`,
	)
	// FIXME: should this be success?
	return ecSuccess
}
