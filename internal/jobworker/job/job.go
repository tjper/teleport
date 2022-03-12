package job

import "context"

type Job struct {
}

func (j Job) StreamOutput(ctx context.Context, stream chan<- []byte) error {
	return nil
}
