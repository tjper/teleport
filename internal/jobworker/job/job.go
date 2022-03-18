package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/tjper/teleport/internal/fsnotify"
	"github.com/tjper/teleport/internal/jobworker"
	"github.com/tjper/teleport/internal/jobworker/output"
	"github.com/tjper/teleport/internal/jobworker/reexec"

	"github.com/google/uuid"
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
		return nil, fmt.Errorf("new job cmd pipe; error: %w", err)
	}
	closers = append(closers, cmdOut)
	closers = append(closers, cmdIn)

	continueOut, continueIn, err := os.Pipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("new job continue pipe; error: %w", err)
	}
	closers = append(closers, continueOut)
	closers = append(closers, continueIn)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	closers = append(closers, watcher)

	shellCmd, err := os.Executable()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("fetch current exec; error: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	executable := exec.CommandContext(ctx, shellCmd, jobworker.Reexec)
	executable.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	executable.ExtraFiles = []*os.File{cmdOut, continueOut}

	id := uuid.New()
	j := &Job{
		mutex:       new(sync.RWMutex),
		ID:          id,
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
	}

	if err := j.setupOutputWatcher(); err != nil {
		cleanup()
		return nil, fmt.Errorf("setup job watcher; error: %w", err)
	}

	logger.Infof("Constructed New Job; ID: %v", id)
	return j, nil
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

	// watcher monitors the output file for changes.
	watcher *fsnotify.Watcher
	// listeners is a map of id and channel pairs. Each channel is notified when
	// watcher detects output file activity.
	listeners map[string]chan struct{}
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

	// TODO: output DNE
	fd, err := os.Open(output.File(j.ID))
	if err != nil {
		return fmt.Errorf("open job output; error: %w", err)
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
			err := j.waitForOutput(ctx)
			if errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
			if err != nil {
				logger.Errorf("waiting for job output; job: %v, error: %v", j.ID, err)
			}
			continue
		}
		/// If EOF and job is not running, return.
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read job output; error: %w", err)
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
// once the Job is no longer running.
func (j Job) cleanup() {
	j.stop()

	if err := j.closeOutputWatcher(); err != nil {
		logger.Errorf("cleanup watcher; error: %v", err)
	}

	closers := []io.Closer{
		j.cmdIn,
		j.cmdOut,
		j.continueIn,
		j.continueOut,
	}

	for _, closer := range closers {
		closer.Close()
	}
}

// start launches the Job.
func (j *Job) start() error {
	logger.Infof("starting Job; ID: %v", j.ID)

	if err := j.exec.Start(); err != nil {
		return fmt.Errorf("start child process; error: %w", err)
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
	logger.Infof("Job running; ID: %v", j.ID)

	return nil
}

// stop terminates the Job.
func (j Job) stop() {
	j.cancel()
}

// newOutputWatcher sets up the watcher that monitors a Job's output file.
// The returned *fsnotify.Watcher should be closed when done being used.
func (j *Job) setupOutputWatcher() error {
	if err := os.WriteFile(output.File(j.ID), nil, output.FileMode); err != nil {
		return fmt.Errorf("setup job output file; job: %v, error: %w", j.ID, err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("new job watcher; job: %v, error: %w", j.ID, err)
	}

	if _, err := watcher.AddWatch(output.File(j.ID)); err != nil {
		watcher.Close()
		return fmt.Errorf("add job watcher; job: %v, error: %w", j.ID, err)
	}

	j.watcher = watcher
	go j.readWatcherEvents()

	return nil
}

// closeOutputWatcher cleans up and closes the Job's output watcher.
func (j Job) closeOutputWatcher() error {
	if err := j.watcher.RemoveWatch(output.File(j.ID)); err != nil {
		logger.Errorf("remove job watcher; job: %v, error: %w", j.ID, err)
	}
	if err := j.watcher.Close(); err != nil {
		return fmt.Errorf("close job watcher; job: %v, error: %w", j.ID, err)
	}
	return nil
}

// readWatcherEvents listens to the output file events stream and notifies
// listeners when events occur.
func (j *Job) readWatcherEvents() {
	for {
		select {
		// TODO: check when this closes
		case <-j.ctx.Done():
			return
		case <-j.watcher.Events:
			j.mutex.RLock()
			for _, listener := range j.listeners {
				listener <- struct{}{}
			}
			j.mutex.RUnlock()
		}
	}
}

// waitForOutput waits for some filesystem event to occur on the Job's output
// file.
func (j *Job) waitForOutput(ctx context.Context) error {
	key := uuid.New().String()
	listen := make(chan struct{})

	j.mutex.Lock()
	j.listeners[key] = listen
	j.mutex.Unlock()

	var err error
	select {
	case <-j.ctx.Done():
		err = j.ctx.Err()
	case <-ctx.Done():
		err = ctx.Err()
	case <-listen:
		err = nil
	}

	j.mutex.Lock()
	delete(j.listeners, key)
	j.mutex.Unlock()

	return err
}

// wait blocks until the Job has exited.
func (j *Job) wait() error {
	var exitErr *exec.ExitError
	err := j.exec.Wait()
	if err != nil && !errors.As(err, &exitErr) {
		return fmt.Errorf("waiting for child; error: %w", err)
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

	logger.Infof("Job no longer waiting; status: %v, exit code: %v", j.Status(), j.ExitCode())
	return nil
}

// signalContinue instructs the Job's executable to continue.
func (j Job) signalContinue() error {
	logger.Infof("Job signal continue to child; ID: %s", j.ID)
	if err := j.continueIn.Close(); err != nil {
		return fmt.Errorf("signal continue to child; error: %w", err)
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

const (
	// noExit is the default process exit code. It indicates a process has not
	// exited, or it was terminated by a signal.
	noExit = -1
)
