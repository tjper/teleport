package cgroup

import (
	"fmt"
	"os"
	"path"
	"strconv"

	"github.com/tjper/teleport/internal/errors"
)

// TODO: determine if cgroup v1 and cgroup v2 must be supported. Current
// approach assumes cgroup v2 is being used.

func newMemoryController(cgroup Cgroup, limit uint64) controller {
	return controller{
		name:   "memory",
		file:   "memory.high",
		value:  strconv.FormatUint(limit, 10),
		cgroup: cgroup,
	}
}

func newCpusController(cgroup Cgroup, cpus float32) controller {
	return controller{
		name:   "cpu",
		file:   "cpu.max",
		value:  strconv.FormatFloat(float64(cpus), 'f', 4, 32),
		cgroup: cgroup,
	}
}

func newIOController(cgroup Cgroup, value string) controller {
	return controller{
		name: "io",
		file: "io.max",
		// TODO: determine devices
		value:  value,
		cgroup: cgroup,
	}
}

type controller struct {
	name  string
	file  string
	value string

	cgroup Cgroup
}

func (c controller) enable() error {
	file := path.Join(c.cgroup.path(), cgroupSubtreeControl)
	fd, err := os.OpenFile(file, os.O_WRONLY, fileMode)
	if err != nil {
		return errors.Wrap(err)
	}
	defer fd.Close()

	_, err = fd.WriteString(fmt.Sprintf("+%s\n", c.name))
	return errors.Wrap(err)
}

func (c controller) create() error {
	file := path.Join(c.cgroup.path(), c.file)
	fd, err := os.OpenFile(file, os.O_WRONLY, fileMode)
	if err != nil {
		return errors.Wrap(err)
	}
	defer fd.Close()

	_, err = fd.WriteString(c.value)
	return errors.Wrap(err)
}

const (
	// controllersSubtreeControl is the name of the file that contains all
	// enabled controllers within a cgroup.
	cgroupSubtreeControl = "cgroup.subtree_control"
	// cpuController is the cgroup2 cpu controller name.
	cpuController = "cpu"
	// memoryController is the cgroup2 memory controller name.
	memoryController = "memory"
	// ioController is the cgroup2 io controller name.
	ioController = "io"
)
