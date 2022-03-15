// Package cli defines the jobworker CLI.
package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

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
	// ecUnrecognized indicates the subcommand was not recognized.
	ecUnrecognized
	// ecCgroupService indicates the cgroup service was not setup properly.
	ecCgroupService
	// ecJobService indicates the job service was not setup properly.
	ecJobService
	// ecTLSConfig indicates the TLS config was not setup properly.
	ecTLSConfig
	// ecListen indicates the jobworker API was unable to listen.
	ecListen
	// ecServe indicates the jobworker API was unable to serve its content.
	ecServe
)

const (
	// serve is the subcommand used to serve the jobworker API.
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

// help outputs a general overview of the jobworker executable to the user.
// The text argument may be used to add a detailed help message.
func help(text string) int {
	var b strings.Builder
	if text != "" {
		_, _ = b.WriteString(fmt.Sprintf("\nNotice: %s", text))
	}

	b.WriteString(
		`

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
`)
	fmt.Fprint(os.Stdout, b.String())
	return ecUnrecognized
}
