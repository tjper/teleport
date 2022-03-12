// Package job provides types for interacting with jobworker jobs, being
// arbitrary commands running on the host system.
package job

import (
	"context"

	"github.com/google/uuid"
)

// NewService creates a new Service intance.
func NewService() *Service {
	return &Service{}
}

// Service facilitates job interactions.
type Service struct {
}

func (s Service) StartJob(ctx context.Context, job Job) error {
	return nil
}

func (s Service) StopJob(ctx context.Context, job Job) error {
	return nil
}

func (s Service) FetchJob(ctx context.Context, id uuid.UUID) (*Job, error) {
	return nil, nil
}
