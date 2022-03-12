package job

import (
	"context"
	"sync"
)

// NewJob creates a new Job instance.
func NewJob(
	userID string,
	cmd Command,
) *Job {
	return &Job{
		mutex:  new(sync.RWMutex),
		userID: userID,
		cmd:    cmd,
	}
}

// Job represents a single arbitrary command and its related entities
// (output, status, etc.).
type Job struct {
	// TODO: replace general Job mutex with field specific mutexs to mitigate
	// unnecessary lock contention.
	mutex *sync.RWMutex

	cmd    Command
	status Status
	userID string
}

// StreamOutput streams Job's output to the passed stream channel.
func (j Job) StreamOutput(ctx context.Context, stream chan<- []byte) error {
	return nil
}

// Status represents the possible statuses of a Job.
type Status string

const (
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
