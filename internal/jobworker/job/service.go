// Package job provides types for interacting with jobworker jobs, being
// arbitrary commands running on the host system.
package job

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/tjper/teleport/internal/jobworker/cgroup"
	"github.com/tjper/teleport/internal/jobworker/output"
	"github.com/tjper/teleport/internal/log"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

// logger is an object for logging package events to stdout.
var logger = log.New(os.Stdout, "job")

var (
	// ErrServiceClosing indicates a StartJob call was made while the Service
	// was closing down.
	ErrServiceClosing = errors.New("service closing")

	// ErrJobAlreadyStarted indicates Service attempted to start a Job that had
	// already started. Highly unlikely for different Job instances as UUIDs are
	// used as identifiers.
	ErrJobAlreadyStarted = errors.New("job already started")

	// ErrJobNotFound indicates the Job is not accessible through the Service.
	ErrJobNotFound = errors.New("job not found")
)

// ICgroupService specifies Service interactions with cgroup.
type ICgroupService interface {
	CreateCgroup(...cgroup.CgroupOption) (*cgroup.Cgroup, error)
	PlaceInCgroup(cgroup.Cgroup, int) error
	RemoveCgroup(uuid.UUID) error
}

// NewService creates a new Service intance.
func NewService(cgroups ICgroupService) (*Service, error) {
	if err := os.MkdirAll(output.Root, output.FileMode); err != nil {
		return nil, fmt.Errorf("mkdir job service output; path: %v, error: %w", output.Root, err)
	}

	return &Service{
		mutex:   new(sync.RWMutex),
		healthy: true,
		jobs:    new(sync.Map),
		cgroups: cgroups,
	}, nil
}

// Service facilitates job interactions.
type Service struct {
	mutex *sync.RWMutex
	// healthy indicates if Service is accepting to jobs to start.
	healthy bool
	// jobs is an mapping of Job.ID keys to *Job instances. The sync.Map type has
	// been used because the data structure is always expanding and deletes never
	// occur.
	jobs    *sync.Map
	cgroups ICgroupService
}

// StartJob starts the job.
func (s *Service) StartJob(_ context.Context, job Job, options ...cgroup.CgroupOption) error {
	if !s.isHealthy() {
		return fmt.Errorf("service unhealthy; err: %w", ErrServiceClosing)
	}

	// Ensure job.ID is not already in use.
	if _, ok := s.jobs.Load(job.ID); ok {
		return fmt.Errorf("%w; job: %v", ErrJobAlreadyStarted, job.ID)
	}
	s.jobs.Store(job.ID, &job)

	cgroup, err := s.cgroups.CreateCgroup(options...)
	if err != nil {
		return err
	}

	if err := job.start(); err != nil {
		return err
	}
	go func() {
		// Goroutine terminates when job is stopped or exits. This can occur
		// because the job executable exits or is terminated. To cleanup all jobs
		// see Service.Close.
		defer job.cleanup()

		if err := job.wait(); err != nil {
			logger.Errorf("%v; job: %v", err, job.ID)
		}

		if err := s.cgroups.RemoveCgroup(cgroup.ID); err != nil {
			logger.Errorf("%v; job: %v, cgroup: %v", err, job.ID, cgroup.ID)
		}
	}()

	// Place Job executable's process within Cgroup.
	if err := s.cgroups.PlaceInCgroup(*cgroup, job.pid()); err != nil {
		job.stop()
		return err
	}

	if err := job.signalContinue(); err != nil {
		job.stop()
		return err
	}

	return nil
}

// StopJob stops the Job associated with the passed job ID.
func (s Service) StopJob(_ context.Context, id uuid.UUID) error {
	job, err := s.loadJob(id)
	if err != nil {
		return err
	}
	if job.Status() != Running {
		return nil
	}

	job.stop()

	return nil
}

// FetchJob retrieves the Job associated with the passed job ID.
func (s Service) FetchJob(_ context.Context, id uuid.UUID) (*Job, error) {
	return s.loadJob(id)
}

// Close releases all Service resources. Close should always be called when
// job.Service is no longer being used.
func (s *Service) Close() error {
	s.mutex.Lock()
	s.healthy = false
	s.mutex.Unlock()

	s.jobs.Range(func(key, value interface{}) bool {
		i, ok := s.jobs.Load(key)
		if !ok {
			return true
		}

		job, ok := i.(*Job)
		if !ok {
			return true
		}

		job.stop()
		return true
	})

	if err := unix.Rmdir(output.Root); err != nil {
		return fmt.Errorf("rmdir job service output; path: %v, error: %w", output.Root, err)
	}

	return nil
}

func (s Service) loadJob(id uuid.UUID) (*Job, error) {
	i, ok := s.jobs.Load(id)
	if !ok {
		return nil, fmt.Errorf("load job; job: %v, err: %w", id, ErrJobNotFound)
	}

	job, ok := i.(*Job)
	if !ok {
		return nil, fmt.Errorf("type check job; job: %v, err: %w", id, ErrJobNotFound)
	}

	return job, nil
}

func (s Service) isHealthy() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.healthy
}
