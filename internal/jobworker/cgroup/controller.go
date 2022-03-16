package cgroup

import (
	"fmt"
	"os"
	"path"
	"strconv"

	"github.com/tjper/teleport/internal/device"
)

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
	value := fmt.Sprintf("%d %d", int(limit), period)

	if err := c.baseController.apply(cpuMax, value); err != nil {
		return err
	}
	return nil
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
	if err := c.baseController.apply(memoryHigh, limit); err != nil {
		return err
	}
	return nil
}

// diskReadBpsController enables and appplies the rbps "io.max" control.
type diskReadBpsController struct {
	baseController
	limit uint64
}

func (c diskReadBpsController) apply() error {
	minors, err := device.ReadDeviceMinors(diskDevices, diskPhysicalMinors)
	if err != nil {
		return err
	}

	for _, minor := range minors {
		value := fmt.Sprintf("%d:%d rbps=%d", diskDevices, minor, c.limit)
		if err := c.baseController.apply(ioMax, value); err != nil {
			return err
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
	minors, err := device.ReadDeviceMinors(diskDevices, diskPhysicalMinors)
	if err != nil {
		return err
	}

	for _, minor := range minors {
		value := fmt.Sprintf("%d:%d wbps=%d", diskDevices, minor, c.limit)
		if err := c.baseController.apply(ioMax, value); err != nil {
			return err
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
	file := path.Join(c.cgroup.path, cgroupSubtreeControl)
	value := fmt.Sprintf("+%s\n", c.name)

	if err := os.WriteFile(file, []byte(value), fileMode); err != nil {
		return fmt.Errorf("enable %s on %s: %w", c.name, file, err)
	}
	return nil
}

// apply sets the value for the specified control in the controller's cgroup.
func (c baseController) apply(control, value string) error {
	file := path.Join(c.cgroup.path, control)

	if err := os.WriteFile(file, []byte(value), fileMode); err != nil {
		return fmt.Errorf("apply %s %s to %s: %w", control, value, file, err)
	}

	return nil
}

const (
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
