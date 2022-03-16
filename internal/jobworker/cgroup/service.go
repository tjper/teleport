// Package cgroup provides types for interaction with Linux cgroups v2.
package cgroup

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tjper/teleport/internal/log"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

// logger is an object for logging package events to stdout.
var logger = log.New(os.Stdout, "cgroups")

// NewService creates a Service instance.
func NewService(options ...ServiceOption) (*Service, error) {
	s := &Service{
		mountPath: mountPath,
	}
	for _, option := range options {
		option(s)
	}

	s.path = path.Join(s.mountPath, jobWorkerBase)

	if err := s.mount(); err != nil {
		return nil, err
	}

	controllers := []string{
		cpu,
		memory,
		io,
	}
	if err := s.enableControllers(controllers); err != nil {
		return nil, err
	}

	return s, nil
}

// Service facilitates cgroup interactions. Service currently only supports
// cgroups v2.
type Service struct {
	mountPath string
	path      string
}

// ServiceOption mutates the Service instance. This is typically used for
// configuration with NewService.
type ServiceOption func(*Service)

// WithMountPath configures the Service instance to mount cgroup2 on mountPath.
func WithMountPath(mountPath string) ServiceOption {
	return func(s *Service) { s.mountPath = mountPath }
}

// CreateCgroup creates a new Service Cgroup. CgroupOptions may be specified to
// configure the Cgroup. On success, the created Cgroup is returned to the
// caller.
func (s Service) CreateCgroup(options ...CgroupOption) (*Cgroup, error) {
	id := uuid.New()
	cgroup := &Cgroup{
		ID:      id,
		service: s,
		path:    path.Join(s.path, id.String()),
	}
	for _, option := range options {
		option(cgroup)
	}

	if err := cgroup.create(); err != nil {
		return nil, err
	}

	return cgroup, nil
}

// PlaceInCgroup places the pid in the Service cgroup specified.
func (s Service) PlaceInCgroup(cgroup Cgroup, pid int) error {
	return cgroup.placePID(pid)
}

// RemoveCgroup removes the jobworker cgroup uniquely identified by the
// specified id.
func (s Service) RemoveCgroup(id uuid.UUID) error {
	cgroup := Cgroup{ID: id, service: s, path: path.Join(s.path, id.String())}

	return cgroup.remove()
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
	file := path.Join(s.mountPath, cgroupProcs)
	fd, err := os.OpenFile(file, os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("open root cgroup: %w", err)
	}
	defer fd.Close()

	for _, pid := range pids {
		if _, err := fd.WriteString(strconv.Itoa(pid)); err != nil {
			return fmt.Errorf("write to root cgroup: %w", err)
		}
	}

	return nil
}

// mount setups the cgroup2 filesystem and creates a cgroup dedicated to
// jobworker cgroups.
func (s Service) mount() error {
	// Ensure path to cgroup2 mount point exists.
	if err := os.MkdirAll(s.mountPath, fileMode); err != nil {
		return fmt.Errorf("mount service %s: %w", s.mountPath, err)
	}

	// If the mount path does not exist or has no entries, mount the cgroup2
	// filesystem.
	entries, err := os.ReadDir(s.mountPath)
	if err != nil || len(entries) == 0 {
		if err := s.mountCgroup2(); err != nil {
			return err
		}
	}

	// cgroup2 filesystem is mounted, ensure jobworker base directory exists.
	if err := os.MkdirAll(s.path, fileMode); err != nil {
		return fmt.Errorf("create jobworker cgroup: %w", err)
	}

	return nil
}

// mountCgroup2 mounts cgroup2 to the Service mountPath.
func (s Service) mountCgroup2() error {
	if err := unix.Mount("none", s.mountPath, "cgroup2", 0, ""); err != nil {
		return fmt.Errorf("mount cgroup2 %s: %w", s.mountPath, err)
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

		parts := strings.Split(path, s.mountPath)
		if len(parts) != 2 {
			return nil
		}

		cgroup2Path := parts[1]
		// Extract the cgroup ID. Skip over cgroup.procs files not created by
		// Service.
		parts = strings.Split(cgroup2Path, string(filepath.Separator))
		if len(parts) != 4 {
			return nil
		}

		cgroupID, err := uuid.Parse(parts[2])
		if err != nil {
			logger.Errorf("non-uuid dir; dir: %s", parts[2])
			return nil
		}

		// Build list of jobworker cgroups on file system.
		cgroups = append(cgroups, cgroupID)

		return nil
	}); err != nil {
		return fmt.Errorf("cleanup jobworker cgroup: %w", err)
	}

	// Remove all jobworker sub cgroups.
	for _, cgroup := range cgroups {
		if err := s.RemoveCgroup(cgroup); err != nil {
			return err
		}
	}

	// Remove root jobworker cgroup.
	if err := unix.Rmdir(s.path); err != nil {
		return fmt.Errorf("rm jobworker cgroup: %w", err)
	}

	return nil
}

// unmount unmounts the cgroup2 filesystem.
func (s Service) unmount() error {
	if err := unix.Unmount(s.mountPath, 0); err != nil {
		return fmt.Errorf("unmount cgroup2: %w", err)
	}
	return nil
}

// enableControllers enables the passed controllers for the root and jobworker
// cgroup.
func (s Service) enableControllers(controllers []string) error {
	if err := enableControllers(s.mountPath, controllers); err != nil {
		return err
	}
	if err := enableControllers(s.path, controllers); err != nil {
		return err
	}
	return nil
}

// enableControllers enables the passed controllers for the cgroup path passed.
func enableControllers(dir string, controllers []string) error {
	fd, err := os.OpenFile(path.Join(dir, cgroupSubtreeControl), os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("open %s subtree_control: %w", dir, err)
	}
	defer fd.Close()

	for _, controller := range controllers {
		_, err := fd.WriteString(fmt.Sprintf("+%s", controller))
		if err != nil {
			return fmt.Errorf("enable %s %s controller: %w", dir, controller, err)
		}
	}

	return nil
}

const (
	// fileMode are the file permissions the jobworker package will use when
	// accessing files.
	fileMode = 0644
	// mountPath is the path the cgroup2 filesystem will be mounted on.
	mountPath = "/cgroup2"
	// jobWorkerBase is the directory name the jobworker cgroups will exist
	// within.
	jobWorkerBase = "jobworker"
)
