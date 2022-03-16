package job

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/tjper/teleport/internal/jobworker"
	"github.com/tjper/teleport/internal/jobworker/output"
	"github.com/tjper/teleport/internal/jobworker/reexec"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// New creates a new Job instance.
func New(
	owner string,
	cmd reexec.Command,
) (*Job, error) {
	var closers []io.Closer
	cleanup := func() {
		for _, closer := range closers {
			closer.Close()
		}
	}

	cmdOut, cmdIn, err := os.Pipe()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	closers = append(closers, cmdOut)
	closers = append(closers, cmdIn)

	continueOut, continueIn, err := os.Pipe()
	if err != nil {
		cleanup()
		return nil, errors.WithStack(err)
	}
	closers = append(closers, continueOut)
	closers = append(closers, continueIn)

	shellCmd, err := os.Executable()
	if err != nil {
		cleanup()
		return nil, errors.WithStack(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	executable := exec.CommandContext(ctx, shellCmd, jobworker.Reexec)
	executable.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	executable.ExtraFiles = []*os.File{cmdOut, continueOut}

	return &Job{
		mutex:       new(sync.RWMutex),
		ID:          uuid.New(),
		Owner:       owner,
		cmd:         cmd,
		status:      Pending,
		exitCode:    noExit,
		ctx:         ctx,
		cancel:      cancel,
		exec:        executable,
		cmdIn:       cmdIn,
		cmdOut:      cmdOut,
		continueIn:  continueIn,
		continueOut: continueOut,
	}, nil
}

// Job represents a single arbitrary command and its related entities
// (output, status, etc.).
type Job struct {
	// TODO: Consider replacing general Job mutex with field specific mutexes to
	// mitigate unnecessary lock contention.
	mutex *sync.RWMutex

	// ID is a unique identifier.
	ID uuid.UUID
	// Owner is the user responsible for Job instance creation.
	Owner string

	cmd      reexec.Command
	status   Status
	exitCode int

	// context.Context is usually utilized at the function level. However, here
	// it is being used to coordinate the cancelling of all async Job resources.
	ctx    context.Context
	cancel context.CancelFunc

	exec                    *exec.Cmd
	cmdIn, cmdOut           io.WriteCloser
	continueIn, continueOut io.WriteCloser
}

// StreamOutput streams Job's output to the passed stream channel in chunks of
// size chunkSize. StreamOutput will return if either of the following
// circumstances occur:
//
// 1) The ctx is cancelled.
// 2) The Job is no longer running and the end of the output is reached.
func (j Job) StreamOutput(ctx context.Context, stream chan<- []byte, chunkSize int) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	fd, err := os.Open(output.File(j.ID))
	if err != nil {
		return errors.WithStack(err)
	}
	go func() {
		<-ctx.Done()
		fd.Close()
	}()

	b := make([]byte, chunkSize)
	for {
		n, err := fd.Read(b)
		// If any bytes were read at all, write to stream.
		if n > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case stream <- b[:n]:
			}
		}
		// If context has been cancelled return to caller.
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		// If EOF and job is running, wait for output from job.
		if errors.Is(err, io.EOF) && j.Status() == Running {
			// TODO: This is a placeholder. Streaming implementation using inotify
			// API will be in another PR. This will be replaced by something like
			// select {
			// case <-ctx.Done():
			//   return ctx.Err()
			// case <-j.watchOutput()
			// }
			time.Sleep(time.Second)
			continue
		}
		/// If EOF and job is not running, return.
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errors.WithStack(err)
		}
	}
}

// Status retrieves the Job status.
func (j Job) Status() Status {
	j.mutex.RLock()
	defer j.mutex.RUnlock()
	return j.status
}

// ExitCode retrieves the Job exit code.
func (j Job) ExitCode() int {
	j.mutex.RLock()
	defer j.mutex.RUnlock()
	return j.exitCode
}

// cleanup releases all resources tied to the Job. cleanup should be called
// once the Job is no longer being used.
func (j Job) cleanup() {
	j.stop()

	closers := []io.Closer{
		j.cmdIn,
		j.cmdOut,
		j.continueIn,
		j.continueOut,
	}

	for _, closer := range closers {
		if err := closer.Close(); err != nil {
			logger.Warnf("closing; error: %v", err)
		}
	}
}

// start launches the Job.
func (j *Job) start() error {
	if err := j.exec.Start(); err != nil {
		return errors.WithStack(err)
	}

	// Write job details to cmdIn pipe. Child process will read and launch
	// grandchild process.
	go func() {
		defer func() {
			if err := j.cmdIn.Close(); err != nil {
				logger.Errorf("closing command pipe; err: %s", err)
			}
		}()

		reexecJob := reexec.Job{
			ID:  j.ID,
			Cmd: j.cmd,
		}
		b, err := json.Marshal(reexecJob)
		if err != nil {
			j.stop()
			return
		}
		if _, err := j.cmdIn.Write(b); err != nil {
			j.stop()
			return
		}
	}()

	j.setStatus(Running)

	return nil
}

// stop terminates the Job.
func (j Job) stop() {
	j.cancel()
}

// wait blocks until the Job has exited.
func (j *Job) wait() error {
	var exitErr *exec.ExitError
	err := j.exec.Wait()
	if err != nil && !errors.As(err, &exitErr) {
		return errors.WithStack(err)
	}

	// Determine nature of process exit.
	switch code := j.exec.ProcessState.ExitCode(); code {
	// If job exit code is -1, process was terminated by a signal.
	case noExit:
		j.setStatus(Stopped)
	default:
		j.setStatus(Exited)
		j.setExitCode(code)
	}

	return nil
}

// signalContinue instructs the Job's executable to continue.
func (j Job) signalContinue() error {
	return errors.WithStack(j.continueIn.Close())
}

// pid retrieves the Job's executable's pid.
func (j Job) pid() int {
	return j.exec.Process.Pid
}

func (j *Job) setStatus(s Status) {
	j.mutex.Lock()
	j.status = s
	j.mutex.Unlock()
}

func (j *Job) setExitCode(code int) {
	j.mutex.Lock()
	j.exitCode = code
	j.mutex.Unlock()
}

// Status represents the possible statuses of a Job.
type Status string

const (
	// Pending indicates the job has been initialized but has not yet started.
	Pending Status = "pending"
	// Running indicates the job is currently running.
	Running Status = "running"
	// Stopped indicates the job has been manually terminated.
	Stopped Status = "stopped"
	// Exited indicates the job exited and returned an exit code.
	Exited Status = "exited"
)

const (
	// noExit is the default process exit code. It indicates a process has not
	// exited, or it was terminated by a signal.
	noExit = -1
)
