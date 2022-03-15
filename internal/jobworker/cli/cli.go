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
	keyFlag    = flag.String("key", "", "path to server private key")
	certFlag   = flag.String("cert", "", "path to server certificate")
	caCertFlag = flag.String("ca_cert", "", "path to CA certificate")
	portFlag   = flag.Int("port", 8080, "port to serve jobworker API")
)

const (
	ecSuccess = iota
	ecUnrecognized
	ecCgroupService
	ecJobService
	ecTLSConfig
	ecListen
	ecServe
)

const (
	serveSub = "serve"
)

// Run is the entrypoint of the jobworker CLI.
func Run() int {
	flag.Parse()

	if len(os.Args) < 2 {
		return help("Too few arguments")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	last := len(os.Args) - 1
	switch v := os.Args[last]; v {
	case serveSub:
		return runServe(ctx)
	case jobworker.Reexec:
		return runReexec(ctx)
	default:
		return help(fmt.Sprintf("Unrecognized subcommand \"%s\".", v))
	}
}

func help(text string) int {
	fmt.Fprintf(
		os.Stdout,
		`
Notice: %s
    
Jobworker launches a grpc API that allows arbitrary commands to be started, 
stopped, retrieved, and streamed.

Usage:
  jobworker [global flags] command

Available Commands:
  serve       Serve jobworker API.
  reexec      Create grandchild process to execute arbitrary command passed 
              from serve process. Should not be called directly.

Global Flags:
  -port       port to serve jobworker API
  -cert       server x509 certificate
  -key        server private key
  -ca_cert    certificate authority cert
`,
		text,
	)
	return ecUnrecognized
}
