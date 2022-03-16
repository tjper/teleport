package cgroup

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

// Cgroup represent a Linux cgroup.
type Cgroup struct {
	// ID is the unique identifier of the cgroup.
	ID uuid.UUID
	// Memory is the "memory.high" bytes limit applied to this cgroup. A zeroed
	// value indicates no limit is set.
	Memory uint64
	// Cpus is the "cpu.max" limit applied to this cgroup. A zeroed value
	// indicates no limit is set.
	Cpus float32
	// DiskWriteBps is the "io.max" bytes written per second limit for 8 block
	// devices applied to this cgroup. A zeroed value indicates no limit is set.
	DiskWriteBps uint64
	// DiskReadBps is the "io.max" bytes read per second limit for 8 block
	// devices applied to this cgroup. A zeroed value indicates no limit is set.
	DiskReadBps uint64

	// service is the Service a Cgroup belongs to.
	service Service

	// path is the file path to the Cgroup
	path string
}

// CgroupOption is a function that mutates Cgroup instances. Typically used
// with cgroups Service.CreateCgroup to create new cgroups Cgroup instances.
type CgroupOption func(*Cgroup)

// WithMemory configures a Cgroup to utilize the specified memory bytes limit.
func WithMemory(limit uint64) CgroupOption {
	return func(c *Cgroup) { c.Memory = limit }
}

// WithCpus configures a Cgroup to utilize the specified cpus limit.
func WithCpus(limit float32) CgroupOption {
	return func(c *Cgroup) { c.Cpus = limit }
}

// WithDiskWriteBps configures a Cgroup to utilize the specified bytes per
// second limit for disk (block 8 devices) writes.
func WithDiskWriteBps(limit uint64) CgroupOption {
	return func(c *Cgroup) { c.DiskWriteBps = limit }
}

// WithDiskReadBps configures a Cgroup to utilize the specified bytes per
// second limit for disk (block 8 devices) reads.
func WithDiskReadBps(limit uint64) CgroupOption {
	return func(c *Cgroup) { c.DiskReadBps = limit }
}

// controller enables and applies cgroup controls.
type controller interface {
	enable() error
	apply() error
}

// create creates a jobworker cgroup.
func (c Cgroup) create() error {
	if err := os.Mkdir(c.path, fileMode); err != nil {
		return fmt.Errorf("create cgroup: %w", err)
	}

	// determine which controllers should be enabled.
	var set []controller
	if c.Memory > 0 {
		set = append(set, newMemoryController(c, c.Memory))
	}
	if c.Cpus > 0 {
		set = append(set, newCPUController(c, c.Cpus))
	}
	if c.DiskWriteBps > 0 {
		set = append(set, newDiskWriteBpsController(c, c.DiskWriteBps))
	}
	if c.DiskReadBps > 0 {
		set = append(set, newDiskReadBpsController(c, c.DiskReadBps))
	}

	for _, controller := range set {
		if err := controller.enable(); err != nil {
			return fmt.Errorf("enable controller: %w", err)
		}
		if err := controller.apply(); err != nil {
			return fmt.Errorf("apply controller: %w", err)
		}
	}

	return nil
}

// placePID adds the specified pid to the cgroup. If the pid exists in another
// cgroup it will be moved to this cgroup.
func (c Cgroup) placePID(pid int) error {
	leaf := uuid.New().String()
	path := filepath.Join(c.path, leaf)
	if err := os.Mkdir(path, fileMode); err != nil {
		return fmt.Errorf("create cgroup leaf: %w", err)
	}

	file := filepath.Join(c.path, leaf, cgroupProcs)
	value := strconv.Itoa(pid)

	if err := os.WriteFile(file, []byte(value), fileMode); err != nil {
		return fmt.Errorf("write cgroup pid: %w", err)
	}

	return nil
}

// remove removes the jobworker cgroup.
func (c Cgroup) remove() error {
	// Read all pids within cgroup.
	pids, err := c.readPids()
	if err != nil {
		return err
	}

	// Move pids to root cgroup. A cgroup must have no dependent pids in its
	// cgroup.procs interface file to be removed.
	if err := c.service.placeInRootCgroup(pids); err != nil {
		return err
	}

	// Remove the cgroup's leaves.
	if err := c.removeLeaves(); err != nil {
		return err
	}

	// Remove the cgroup's jobworker directory.
	if err := unix.Rmdir(c.path); err != nil {
		return fmt.Errorf("remove cgroup: %w", err)
	}

	return nil
}

// readPids retrieves all pids that belong to the jobworker cgroup.
func (c Cgroup) readPids() ([]int, error) {
	var pids []int
	if err := filepath.WalkDir(c.path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			logger.Errorf("reading cgroup pids: %s", err)
			return nil
		}

		// Filter out all paths that are not cgroup.procs
		if !d.Type().IsRegular() || d.Name() != cgroupProcs {
			return nil
		}

		// Handle path relative to cgroup path.
		parts := strings.Split(path, c.path)
		if len(parts) != 2 {
			return nil
		}

		leafPath := parts[1]
		// Ensure cgroup.procs belongs to leaf cgroup.
		parts = strings.Split(leafPath, string(filepath.Separator))
		if len(parts) != 3 {
			return nil
		}

		leafPids, err := readLeafPids(path)
		if err != nil {
			logger.Errorf("reading leaf pids; path: %v, error: %v", path, err)
		}
		pids = append(pids, leafPids...)

		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk cgroup leaf cgroup.procs: %w", err)
	}

	return pids, nil
}

func (c Cgroup) removeLeaves() error {
	var leaves []uuid.UUID
	if err := filepath.WalkDir(c.path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			logger.Errorf("reading cgroup leaves: %v", err)
			return nil
		}

		// Filter out all paths that are not cgroup.procs
		if !d.Type().IsRegular() || d.Name() != cgroupProcs {
			return nil
		}

		// Handle path relative to cgroup path.
		parts := strings.Split(path, c.path)
		if len(parts) != 2 {
			return nil
		}
		leafPath := parts[1]

		// Extract the leaf cgroup ID. Skip over cgroup.procs that are not on
		// leaves.
		parts = strings.Split(leafPath, string(filepath.Separator))
		if len(parts) != 3 {
			return nil
		}

		leafCgroupID, err := uuid.Parse(parts[1])
		if err != nil {
			logger.Errorf("non-uuid dir; dir: %s", parts[2])
			return nil
		}

		// Build list of leaf cgroups in cgroup.
		leaves = append(leaves, leafCgroupID)
		return nil
	}); err != nil {
		return fmt.Errorf("walk cgroup leaves: %w", err)
	}

	for _, leaf := range leaves {
		path := filepath.Join(c.path, leaf.String())
		if err := unix.Rmdir(path); err != nil {
			return fmt.Errorf("rm leaf cgroup; path: %s, error: %v", path, err)
		}
	}
	return nil
}

// readLeafPids retrieves all pids that belong to leaf cgroup.
func readLeafPids(path string) ([]int, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read leaf cgroup pids: %w", err)
	}
	defer fd.Close()

	var pids []int
	procs := bufio.NewScanner(fd)
	for procs.Scan() {
		pid, err := strconv.Atoi(procs.Text())
		if err != nil {
			return nil, fmt.Errorf("scan leaf cgroup.procs pids atoi: %w", err)
		}
		pids = append(pids, pid)
	}
	if procs.Err() != nil {
		return nil, fmt.Errorf("scan leaf cgroup.procs pids: %w", err)
	}

	return pids, nil
}

const (
	// cgroupProcs is the name of the file that contains all processes within a
	// cgroup.
	cgroupProcs = "cgroup.procs"
)
