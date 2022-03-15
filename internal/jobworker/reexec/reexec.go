// Package reexec provides an API for launching arbitrary commands in a
// jobworker child process.
package reexec

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/tjper/teleport/internal/jobworker/output"
	"github.com/tjper/teleport/internal/log"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// logger is an object for logging package events to stdout.
var logger = log.New(os.Stdout, "reexec")

var (
	// ErrCommandPipeNotFound indicates that the parent process did not properly
	// configure the command pipe and pass it to the child process.
	ErrCommandPipeNotFound = errors.New("command pipe not found")
	// ErrContinuePipeNotFound indicates that the parent process did not properly
	// configure the continue pipe and pass it to the child process.
	ErrContinuePipeNotFound = errors.New("continue pipe not found")
)

var (
	// errExpectedEOF indicates the read operation expected an io.EOF error, but
	// no error was returned.
	errExpectedEOF = errors.New("expected EOF")
)

const (
	// CommandSuccess indicates the reexec exuction of a command completed
	// successfully.
	CommandSuccess = 0
	// CommandFailure indicates the reexec execution of a command failed before
	// being executed; in the setup phase.
	CommandFailure = 100
)

// Job is a Job passed by the parent to be executed by the child.
type Job struct {
	// ID is a unique identifier for the Job. The parent and child share the
	// Job ID for each unique Job.
	ID uuid.UUID
	// Cmd is the arbitrary command to run as part of this Job.
	Cmd Command
}

// Command represents a shell command.
type Command struct {
	// Name is the leading name of the command.
	Name string
	// Args are the arguments of the command.
	Args []string
}

// Exec utilizes the piped data from the parent process to build and run a
// arbitrary command on the host system.
func Exec(ctx context.Context) (int, error) {
	// Parent process has set /proc/self/fd/3 to the command pipe receiver.
	cmdfd := os.NewFile(uintptr(3), "/proc/self/fd/3")
	if cmdfd == nil {
		return CommandFailure, ErrCommandPipeNotFound
	}

	// Parent process has set the /proc/self/fd/4 to the continue pipe receiver.
	contfd := os.NewFile(uintptr(4), "/proc/self/fd/4")
	if contfd == nil {
		return CommandFailure, ErrContinuePipeNotFound
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(cmdfd); err != nil {
		return CommandFailure, errors.WithStack(err)
	}
	var job Job
	if err := json.Unmarshal(buf.Bytes(), &job); err != nil {
		return CommandFailure, errors.WithStack(err)
	}

	// Create log file for stdout and stderr output.
	outfd, err := os.OpenFile(output.File(job.ID), os.O_CREATE|os.O_WRONLY, output.FileMode)
	if err != nil {
		return CommandFailure, errors.WithStack(err)
	}
	defer func() {
		if err := outfd.Close(); err != nil {
			logger.Errorf("closing output fd; error: %s", err)
		}
	}()

	// Build command to be run on host system.
	cmd := exec.Command(job.Cmd.Name, job.Cmd.Args...)
	cmd.Stdout = outfd
	cmd.Stderr = outfd

	// Wait for continue signal from parent process. This will be sent once
	// process has been placed in the appropriate cgroup.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := waitForContinue(ctx, contfd); err != nil {
		return CommandFailure, errors.WithStack(err)
	}

	if err := cmd.Start(); err != nil {
		return CommandFailure, errors.WithStack(err)
	}

	err = cmd.Wait()
	return exitCode(err), nil
}

func exitCode(err error) int {
	if err == nil {
		return CommandSuccess
	}

	exitError := new(exec.ExitError)
	if errors.As(err, &exitError) {
		status, ok := exitError.Sys().(syscall.WaitStatus)
		if !ok {
			return CommandFailure
		}
		return status.ExitStatus()
	}
	return CommandFailure
}

// waitForContinue waits for EOF to be received from fd. The parent process
// will close fd when this process may continue.
func waitForContinue(ctx context.Context, fd io.ReadCloser) error {
	go func() {
		<-ctx.Done()
		if err := fd.Close(); err != nil {
			logger.Errorf("closing continue pipe; err: %s", err)
		}
	}()

	b := make([]byte, 1)
	_, err := fd.Read(b)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return errors.WithStack(err)
	}
	return errExpectedEOF
}
