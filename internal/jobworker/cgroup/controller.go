package cgroup

import (
	"fmt"
	"os"
	"path"
	"strconv"

	"github.com/tjper/teleport/internal/errors"
)

type controller interface {
	enable() error
	apply() error
}

func newMemoryController(cgroup Cgroup, limit uint64) *memoryController {
	return &memoryController{
		baseController: baseController{name: memory, cgroup: cgroup},
		limit:          limit,
	}
}

func newCPUController(cgroup Cgroup, cpus float32) *cpuController {
	return &cpuController{
		baseController: baseController{name: cpu, cgroup: cgroup},
		cpus:           cpus,
	}
}

func newDiskReadBps(cgroup Cgroup, limit uint64) *diskReadBpsController {
	return &diskReadBpsController{
		baseController: baseController{name: io, cgroup: cgroup},
		limit:          limit,
	}
}

func newDiskWriteBps(cgroup Cgroup, limit uint64) *diskWriteBpsController {
	return &diskWriteBpsController{
		baseController: baseController{name: io, cgroup: cgroup},
		limit:          limit,
	}
}

type baseController struct {
	name   string
	cgroup Cgroup
}

func (c baseController) enable() error {
	file := path.Join(c.cgroup.path(), cgroupSubtreeControl)
	fd, err := os.OpenFile(file, os.O_WRONLY, fileMode)
	if err != nil {
		return errors.Wrap(err)
	}
	defer fd.Close()

	_, err = fd.WriteString(fmt.Sprintf("+%s\n", c.name))
	return errors.Wrap(err)
}

func (c baseController) apply(control, value string) error {
	file := path.Join(c.cgroup.path(), control)
	fd, err := os.OpenFile(file, os.O_WRONLY, fileMode)
	if err != nil {
		return errors.Wrap(err)
	}
	defer fd.Close()

	_, err = fd.WriteString(value)
	return errors.Wrap(err)
}

type cpuController struct {
	baseController
	cpus float32
}

func (c cpuController) apply() error {
	const (
		period = 100000
	)
	limit := c.cpus * period
	value := fmt.Sprintf("%f %d", limit, period)

	return errors.Wrap(c.baseController.apply(cpuMax, value))
}

type memoryController struct {
	baseController
	limit uint64
}

func (c memoryController) apply() error {
	limit := strconv.FormatUint(c.limit, 10)
	return errors.Wrap(c.baseController.apply(memoryHigh, limit))
}

type diskReadBpsController struct {
	baseController
	limit uint64
}

func (c diskReadBpsController) apply() error {
	for minor := diskMinMinor; minor <= diskMaxMinor; minor += diskMinorPartition {
		value := fmt.Sprintf("%d:%d rbps=%d", diskDevices, minor, c.limit)
		if err := c.baseController.apply(ioMax, value); err != nil {
			return errors.Wrap(err)
		}
	}
	return nil
}

type diskWriteBpsController struct {
	baseController
	limit uint64
}

func (c diskWriteBpsController) apply() error {
	for minor := diskMinMinor; minor <= diskMaxMinor; minor += diskMinorPartition {
		value := fmt.Sprintf("%d:%d wbps=%d", diskDevices, minor, c.limit)
		if err := c.baseController.apply(ioMax, value); err != nil {
			return errors.Wrap(err)
		}
	}
	return nil
}

const (
	// devices is the dev filesystem.
	devices = "/dev"
	// diskDevices is major number for disk devices.
	diskDevices = 8
	// diskMaxMinor is the maximum disk device minor number.
	diskMaxMinor = 240
	// diskMinMinor is the minimum disk device minor number.
	diskMinMinor = 0
	// diskMinorPartition is the numbers between disk device minor numbers.
	diskMinorPartition = 16
	// controllersSubtreeControl is the name of the file that contains all
	// enabled controllers within a cgroup.
	cgroupSubtreeControl = "cgroup.subtree_control"
	// cpu is the cgroup cpu controller name.
	cpu = "cpu"
	// memory is the cgroup memory controller name.
	memory = "memory"
	// io is the cgroup io controller name.
	io = "io"
	// memoryHigh is the memory.high cgroup control.
	memoryHigh = "memory.high"
	// cpuMax is the cpu.max cgroup control.
	cpuMax = "cpu.max"
	// ioMax is the io.max cgroup control.
	ioMax = "io.max"
)
