// Package job provides types for interacting with jobworker jobs, being
// arbitrary commands running on the host system.
package job

import "github.com/google/uuid"

// NewService creates a new Service intance.
func NewService() *Service {
	return &Service{}
}

// Service facilitates job interactions.
type Service struct {
}

func (s Service) StartJob(job Job) error {
	return nil
}

func (s Service) StopJob(job Job) error {
	return nil
}

func (s Service) FetchJob(id uuid.UUID) (*Job, error) {
	return nil, nil
}
