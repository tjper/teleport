// Package device provides an API composed of utilities for interacting with
// /dev.
package device

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// ErrPartitionSize indicates more than one int was passed.
var ErrPartitionSize = errors.New("partion size may only contain one item")

// ReadDeviceMinors retrieves the device minors of the specified major.
// Specify a paritonSize if partion minor numbers should be returned.
func ReadDeviceMinors(major uint32, partitionSize ...int) ([]uint32, error) {
	if len(partitionSize) > 1 {
		return nil, ErrPartitionSize
	}

	var minors []uint32
	if err := filepath.WalkDir(devices, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.Type() != fs.ModeDevice {
			return nil
		}

		var stats unix.Stat_t
		if err := unix.Stat(path, &stats); err != nil {
			return nil
		}

		if unix.Major(stats.Rdev) != major {
			return nil
		}

		minor := unix.Minor(stats.Rdev)
		if len(partitionSize) == 1 && minor%uint32(partitionSize[0]) != 0 {
			return nil
		}

		minors = append(minors, minor)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("read disk device minors: %w", err)
	}

	return minors, nil
}

const (
	// devices is the dev filesystem.
	devices = "/dev"
)
