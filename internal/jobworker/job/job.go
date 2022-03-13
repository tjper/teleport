package job

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	ierrors "github.com/tjper/teleport/internal/errors"
	"github.com/tjper/teleport/internal/jobworker/log"
	"github.com/tjper/teleport/internal/jobworker/watch"

	"github.com/google/uuid"
)

// NewJob creates a new Job instance.
func NewJob(
	owner string,
	cmd Command,
) (*Job, error) {
	cmdOut, cmdIn, err := os.Pipe()
	if err != nil {
		return nil, ierrors.Wrap(err)
	}

	continueOut, continueIn, err := os.Pipe()
	if err != nil {
		return nil, ierrors.Wrap(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	executable := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	executable.SysProcAttr.Setpgid = true
	executable.ExtraFiles = []*os.File{
		cmdOut,
		continueOut,
	}

	id := uuid.New()
	watcher := watch.NewModWatcher(log.File(id))

	return &Job{
		mutex:       new(sync.RWMutex),
		ID:          id,
		owner:       owner,
		cmd:         cmd,
		status:      Pending,
		exitCode:    noExit,
		exec:        executable,
		cmdIn:       cmdIn,
		cmdOut:      cmdOut,
		continueIn:  continueIn,
		continueOut: continueOut,
		cancel:      cancel,
		watcher:     *watcher,
	}, nil
}

// Job represents a single arbitrary command and its related entities
// (output, status, etc.).
type Job struct {
	// TODO: Consider replacing general Job mutex with field specific mutexes to
	// mitigate unnecessary lock contention.
	mutex *sync.RWMutex

	ID     uuid.UUID
	cmd    Command
	status Status
	// exitCode defaults to noExit, indicating the job has not exited.
	exitCode int
	owner    string

	exec                    *exec.Cmd
	cmdIn, cmdOut           io.WriteCloser
	continueIn, continueOut io.WriteCloser
	cancel                  context.CancelFunc
	watcher                 watch.ModWatcher
}

// StreamOutput streams Job's output to the passed stream channel. StreamOutput
// will return if either of the following circumstances occur:
//
// 1) The ctx is cancelled.
// 2) The Job is no longer running and the end of the output is reached.
func (j Job) StreamOutput(ctx context.Context, stream chan<- []byte) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	fd, err := os.Open(log.File(j.ID))
	if err != nil {
		return ierrors.Wrap(err)
	}
	go func() {
		<-ctx.Done()
		fd.Close()
	}()

	b := make([]byte, readBufferSize)
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
			if err := j.waitUntilOutput(ctx); err != nil {
				return ierrors.Wrap(err)
			}
		}
		/// If EOF and job is not running, return.
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return ierrors.Wrap(err)
		}
	}
}

// Owner retrieves the Job owner.
func (j Job) Owner() string { return j.owner }

// Status retrieves the Job status.
func (j Job) Status() Status {
	j.mutex.RLock()
	defer j.mutex.RUnlock()
	return j.status
}

// cleanup releases all resources tied to the Job. cleanup should be called
// once the Job is no longer being used.
func (j Job) cleanup() error {
	j.cancel()

	// TODO: Ensure this works as expected, maybe create an error chain type
	// and test.
	var gerr error
	check := func(err error) {
		if gerr == nil {
			gerr = ierrors.Wrap(err)
		}
	}

	check(j.cmdIn.Close())
	check(j.cmdOut.Close())
	check(j.continueIn.Close())
	check(j.continueOut.Close())
	if gerr != nil {
		return gerr
	}

	return nil
}

// start launches the Job.
func (j Job) start() error {
	if err := j.exec.Start(); err != nil {
		return ierrors.Wrap(err)
	}

	j.setStatus(Running)

	return nil
}

// stop terminates the Job.
func (j Job) stop() {
	j.cancel()
}

// wait blocks until the Job has exited.
func (j Job) wait() error {
	if err := j.exec.Wait(); err != nil {
		return ierrors.Wrap(err)
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
	return ierrors.Wrap(j.continueIn.Close())
}

// waitUntilOutput blocks until the Job watcher indicates the Job output has
// been modified.
func (j Job) waitUntilOutput(ctx context.Context) error {
	if err := j.watcher.WaitUntil(ctx); err != nil {
		return ierrors.Wrap(err)
	}
	return nil
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

// Command represents a shell command.
type Command struct {
	// Name is the leading name of the command.
	Name string
	// Args are the arguments of the command.
	Args []string
}

const (
	// noExit is the default process exit code. It indicates a process has not
	// exited, or it was terminated by a signal.
	noExit = -1

	// tick is the default modification watcher interval.
	tick = time.Second

	// readBufferSize is the default buffer size for streaming a job's output.
	readBufferSize = 512
)
