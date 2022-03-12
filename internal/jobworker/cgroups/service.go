// Package cgroups provides types for interaction with Linux cgroups v2.
package cgroups

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tjper/teleport/internal/errors"
	"github.com/tjper/teleport/internal/log"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

// logger is an object for logging package events to stdout.
var logger = log.New(os.Stdout, "cgroups")

// NewService creates a Service instance.
func NewService() (*Service, error) {
	s := &Service{
		// path has a random ID to ensure Service instance bases do not collide.
		path: path.Join(mountPath, jobWorkerBase),
	}

	if err := s.mount(); err != nil {
		return nil, err
	}

	return s, nil
}

// Service facilitates cgroup interactions. Service currently only supports
// cgroups v2.
type Service struct {
	path string
}

// CreateCgroup creates a new Service Cgroup. CgroupOptions may be specified to
// configure the Cgroup. On success, the created Cgroup is returned to the
// caller.
func (s Service) CreateCgroup(options ...CgroupOption) (*Cgroup, error) {
	cgroup := &Cgroup{
		ID:      uuid.New(),
		service: s,
	}
	for _, option := range options {
		option(cgroup)
	}

	if err := cgroup.create(); err != nil {
		return nil, errors.Wrap(err)
	}

	return cgroup, nil
}

// PlaceInCgroup places the pid in the Service cgroup specified.
func (s Service) PlaceInCgroup(cgroup Cgroup, pid int) error {
	return errors.Wrap(cgroup.placePID(pid))
}

// RemoveCgroup removes the jobworker cgroup uniquely identified by the
// specified id.
func (s Service) RemoveCgroup(id uuid.UUID) error {
	cgroup := Cgroup{ID: id, service: s}

	return errors.Wrap(cgroup.remove())
}

// Cleanup removes all jobworker Service resources. Whenever a Service instance
// is used, Cleanup should always be called before application close.
func (s Service) Cleanup() error {
	if err := s.cleanup(); err != nil {
		return err
	}

	if err := s.unmount(); err != nil {
		return err
	}

	return nil
}

// placeInRootCgroup moves the pids into the root cgroup.
func (s Service) placeInRootCgroup(pids []int) error {
	file := path.Join(mountPath, cgroupProcs)
	fd, err := os.OpenFile(file, os.O_WRONLY, fileMode)
	if err != nil {
		return errors.Wrap(err)
	}
	defer fd.Close()

	for _, pid := range pids {
		if _, err := fd.WriteString(strconv.Itoa(pid)); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

// mount setups the cgroup2 filesystem and creates a cgroup dedicated to
// jobworker cgroups.
func (s Service) mount() error {
	// Ensure path to cgroup2 mount point exists.
	if err := os.MkdirAll(mountPath, fileMode); err != nil {
		return errors.Wrap(err)
	}

	// If the mount path does not exist or has no entries, mount the cgroup
	// filesystem, and make base directory for jobworker cgroups.
	entries, err := os.ReadDir(mountPath)
	if err != nil || len(entries) == 0 {
		goto mount
	}

	// cgroup2 filesystem is mounted, ensure jobworker base directory exists and
	// return.
	if err := os.MkdirAll(s.path, fileMode); err != nil {
		return errors.Wrap(err)
	}
	return nil

mount:
	if err := unix.Mount("none", mountPath, "cgroup2", 0, ""); err != nil {
		return errors.Wrap(err)
	}

	// create jobworker base directory for jobworker cgroups.
	if err := os.MkdirAll(s.path, fileMode); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

// cleanup walks the Service base directory, moving all jobworker pids into the
// root cgroup and removing the each cgroup directory.
func (s Service) cleanup() error {
	var cgroups []uuid.UUID

	if err := filepath.WalkDir(s.path, func(path string, d fs.DirEntry, err error) error {
		// In the event an error occurred while walking, log and continue cleanup.
		if err != nil {
			logger.Errorf("cleanup walking dir: %s", err)
			return nil
		}

		// Filter out all paths that are not cgroup.procs
		if !d.Type().IsRegular() || d.Name() != cgroupProcs {
			return nil
		}

		// Extract the cgroup ID. Skip over cgroup.procs files not created by
		// Service.
		parts := strings.Split(path, string(filepath.Separator))
		if len(parts) != 5 {
			return nil
		}

		cgroupID, err := uuid.Parse(parts[3])
		if err != nil {
			logger.Errorf("non-uuid dir; dir: %s", parts[3])
			return nil
		}

		// Build list of jobworker cgroups on file system.
		cgroups = append(cgroups, cgroupID)

		return nil
	}); err != nil {
		return errors.Wrap(err)
	}

	// Remove all jobworker sub cgroups.
	for _, cgroup := range cgroups {
		if err := s.RemoveCgroup(cgroup); err != nil {
			return err
		}
	}

	// Remove root jobworker cgroup.
	if err := unix.Rmdir(s.path); err != nil {
		return err
	}

	return nil
}

// unmount unmounts the cgroup2 filesystem.
func (s Service) unmount() error {
	return errors.Wrap(unix.Unmount(mountPath, 0))
}

const (
	// fileMode are the file permissions the jobworker package will use when
	// accessing files.
	fileMode = 0555
	// mountPath is the path the cgroup2 filesystem will be mounted on.
	mountPath = "/cgroup2"
	// jobWorkerBase is the directory name the jobworker cgroups will exist
	// within.
	jobWorkerBase = "jobworker"
)
