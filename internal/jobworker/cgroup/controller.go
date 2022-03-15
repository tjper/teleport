package cgroup

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/tjper/teleport/internal/errors"
	"golang.org/x/sys/unix"
)

// controller enables and applies cgroup controls
type controller interface {
	enable() error
	apply() error
}

// newCpuController creates a cpuController instance.
func newCPUController(cgroup Cgroup, cpus float32) *cpuController {
	return &cpuController{
		baseController: baseController{name: cpu, cgroup: cgroup},
		cpus:           cpus,
	}
}

// cpuController enables and applies the "cpu.max" control.
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

// newMemoryController creates a memoryController instance.
func newMemoryController(cgroup Cgroup, limit uint64) *memoryController {
	return &memoryController{
		baseController: baseController{name: memory, cgroup: cgroup},
		limit:          limit,
	}
}

// memoryController enabled and applies the "memory.high" control.
type memoryController struct {
	baseController
	limit uint64
}

func (c memoryController) apply() error {
	limit := strconv.FormatUint(c.limit, 10)
	return errors.Wrap(c.baseController.apply(memoryHigh, limit))
}

// diskReadBpsController enables and appplies the rbps "io.max" control.
type diskReadBpsController struct {
	baseController
	limit uint64
}

func (c diskReadBpsController) apply() error {
	minors, err := readDiskDeviceMinors()
	if err != nil {
		return errors.Wrap(err)
	}

	for _, minor := range minors {
		value := fmt.Sprintf("%d:%d rbps=%d", diskDevices, minor, c.limit)
		if err := c.baseController.apply(ioMax, value); err != nil {
			return errors.Wrap(err)
		}
	}
	return nil
}

// newDiskReadBpsController creates a diskReadBpsController instance.
func newDiskReadBpsController(cgroup Cgroup, limit uint64) *diskReadBpsController {
	return &diskReadBpsController{
		baseController: baseController{name: io, cgroup: cgroup},
		limit:          limit,
	}
}

// newDiskWriteBpsController creates a diskWriteBpsController instance.
func newDiskWriteBpsController(cgroup Cgroup, limit uint64) *diskWriteBpsController {
	return &diskWriteBpsController{
		baseController: baseController{name: io, cgroup: cgroup},
		limit:          limit,
	}
}

// diskReadBpsController enables and appplies the wbps "io.max" control.
type diskWriteBpsController struct {
	baseController
	limit uint64
}

func (c diskWriteBpsController) apply() error {
	minors, err := readDiskDeviceMinors()
	if err != nil {
		return errors.Wrap(err)
	}

	for _, minor := range minors {
		value := fmt.Sprintf("%d:%d wbps=%d", diskDevices, minor, c.limit)
		if err := c.baseController.apply(ioMax, value); err != nil {
			return errors.Wrap(err)
		}
	}
	return nil
}

// baseController owns controller logic shared by most controller implementations.
type baseController struct {
	name   string
	cgroup Cgroup
}

// enable enables a controller by writing to the cgroup.subtree_control file of
// the cgroup.
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

// apply sets the value for the specified control in the controller's cgroup.
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

// readDiskDeviceMinors retrieves the physical disk device minors of disk
// (8 block) devices.
func readDiskDeviceMinors() ([]uint32, error) {
	var minors []uint32
	if err := filepath.WalkDir(devices, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			logger.Errorf("walking devices; error: %v", err)
			return nil
		}

		if d.Type() != fs.ModeDevice {
			return nil
		}

		stats := unix.Stat_t{}
		if err := unix.Stat(path, &stats); err != nil {
			logger.Errorf("stats error: %s", err)
			return nil
		}

		if unix.Major(stats.Rdev) != diskDevices {
			return nil
		}

		minor := unix.Minor(stats.Rdev)
		if minor%diskPhysicalMinors != 0 {
			return nil
		}

		minors = append(minors, minor)
		return nil
	}); err != nil {
		return nil, errors.Wrap(err)
	}

	return minors, nil
}

const (
	// devices is the dev filesystem.
	devices = "/dev"
	// diskDevices is major number for disk devices.
	diskDevices = 8
	// diskPhysicalMinors is the numbers between disk device minor numbers.
	diskPhysicalMinors = 16
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
