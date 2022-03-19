package main

import (
	"os"

	"github.com/tjper/teleport/internal/jobworker/cli"
)

func main() {
	os.Exit(cli.Run())
}
