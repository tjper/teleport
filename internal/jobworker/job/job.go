package job

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/tjper/teleport/internal/errors"

	"github.com/google/uuid"
)

// NewJob creates a new Job instance.
func NewJob(
	owner string,
	cmd Command,
) (*Job, error) {
	cmdOut, cmdIn, err := os.Pipe()
	if err != nil {
		return nil, errors.Wrap(err)
	}

	continueOut, continueIn, err := os.Pipe()
	if err != nil {
		return nil, errors.Wrap(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	executable := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	executable.SysProcAttr.Setpgid = true
	executable.ExtraFiles = []*os.File{
		cmdOut,
		continueOut,
	}

	return &Job{
		mutex:       new(sync.RWMutex),
		ID:          uuid.New(),
		owner:       owner,
		cmd:         cmd,
		status:      Pending,
		exitCode:    -1,
		exec:        executable,
		cmdIn:       cmdIn,
		cmdOut:      cmdOut,
		continueIn:  continueIn,
		continueOut: continueOut,
		cancel:      cancel,
	}, nil
}

// Job represents a single arbitrary command and its related entities
// (output, status, etc.).
type Job struct {
	// TODO: replace general Job mutex with field specific mutexes to mitigate
	// unnecessary lock contention.
	mutex *sync.RWMutex

	ID     uuid.UUID
	cmd    Command
	status Status
	// exitCode defaults to -1, indicating the job has not exited.
	exitCode int
	owner    string

	exec                    *exec.Cmd
	cmdIn, cmdOut           io.WriteCloser
	continueIn, continueOut io.WriteCloser
	cancel                  context.CancelFunc
}

// StreamOutput streams Job's output to the passed stream channel. StreamOutput
// will return if either of the following circumstances occur:
//
// 1) The ctx is cancelled.
// 2) The Job is no longer running and the end of the output is reached.
func (j Job) StreamOutput(ctx context.Context, stream chan<- []byte) error {

	return nil
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
			gerr = errors.Wrap(err)
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
		return errors.Wrap(err)
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
		return errors.Wrap(err)
	}

	// Determine nature of process exit.
	switch code := j.exec.ProcessState.ExitCode(); code {
	// If job exit code is -1, process was terminated by a signal.
	case -1:
		j.setStatus(Stopped)
	default:
		j.setStatus(Exited)
		j.setExitCode(code)
	}

	return nil
}

// signalContinue instructs the Job's executable to continue.
func (j Job) signalContinue() error {
	return errors.Wrap(j.continueIn.Close())
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
