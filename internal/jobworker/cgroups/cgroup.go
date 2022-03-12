package cgroups

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strconv"

	"github.com/google/uuid"
	"github.com/tjper/teleport/internal/errors"
	"golang.org/x/sys/unix"
)

// Cgroup represent a Linux cgroup.
type Cgroup struct {
	// ID is the unique identifier of the cgroup.
	ID uuid.UUID
	// Memory is the "memory.high" bytes limit applied to this cgroup. An empty
	// value indicates no limit is set.
	Memory uint64
	// Cpus is the "cpu.max" limit applied to this cgroup. An empty value
	// indicates no limit is set.
	Cpus float32
	// DiskWriteBps is the "io.max" bytes written per second limit for 8 block
	// devices applied to this cgroup. An empty value indicates no limit is set.
	DiskWriteBps uint64
	// DiskReadBps is the "io.max" bytes read per second limit for 8 block
	// devices applied to this cgroup. An empty value indicates no limit is set.
	DiskReadBps uint64

	// service is the Service a Cgroup belongs to.
	service Service
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

// create creates a jobworker cgroup.
func (c Cgroup) create() error {
	if err := os.Mkdir(c.path(), fileMode); err != nil {
		return errors.Wrap(err)
	}

	// determine which controllers should be enabled.
	var set []controller
	if c.Memory > 0 {
		set = append(set, newMemoryController(c, c.Memory))
	}
	if c.Cpus > 0 {
		set = append(set, newCpusController(c, c.Cpus))
	}
	if c.DiskWriteBps > 0 {
		set = append(set, newIOController(c, fmt.Sprintf("8:16 wbps=%d", c.DiskWriteBps)))
	}
	if c.DiskReadBps > 0 {
		set = append(set, newIOController(c, fmt.Sprintf("8:16 rbps=%d", c.DiskReadBps)))
	}

	for _, controller := range set {
		if err := controller.enable(); err != nil {
			return errors.Wrap(err)
		}
		if err := controller.create(); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

// placePID adds the specified pid to the cgroup. If the pid exists in another
// cgroup it will be moved to this cgroup.
func (c Cgroup) placePID(pid int) error {
	file := path.Join(c.path(), cgroupProcs)
	fd, err := os.OpenFile(file, os.O_WRONLY, fileMode)
	if err != nil {
		return errors.Wrap(err)
	}
	defer fd.Close()

	_, err = fd.WriteString(strconv.Itoa(pid))
	return errors.Wrap(err)
}

// remove removes the jobworker cgroup.
func (c Cgroup) remove() error {
	// Read all pids within cgroup.
	pids, err := c.readPids()
	if err != nil {
		return errors.Wrap(err)
	}

	// Move pids to root cgroup. A cgroup must have no dependent pids in its
	// cgroup.procs interface file to be removed.
	if err := c.service.placeInRootCgroup(pids); err != nil {
		return errors.Wrap(err)
	}

	// Remove the cgroup's jobworker directory.
	err = unix.Rmdir(c.path())

	return errors.Wrap(err)
}

func (c Cgroup) path() string {
	return path.Join(c.service.path, c.ID.String())
}

// readPids retrieves all pids that belong to the jobworker cgroup.
func (c Cgroup) readPids() ([]int, error) {
	file := path.Join(c.path(), cgroupProcs)
	fd, err := os.Open(file)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	defer fd.Close()

	var pids []int
	procs := bufio.NewScanner(fd)
	for procs.Scan() {
		pid, err := strconv.Atoi(procs.Text())
		if err != nil {
			return nil, errors.Wrap(err)
		}
		pids = append(pids, pid)
	}
	if procs.Err() != nil {
		return nil, errors.Wrap(procs.Err())
	}

	return pids, nil
}

const (
	// cgroupProcs is the name of the file that contains all processes within a
	// cgroup.
	cgroupProcs = "cgroup.procs"
)
