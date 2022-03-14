package cgroup

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"reflect"
	"testing"
)

func TestCleanup(t *testing.T) {
	if !isRoot() {
		t.Skip("must be root to run")
	}

	service, err := NewService()
	if err != nil {
		t.Error(err)
		return
	}

	if _, err := os.Stat(service.path); err != nil {
		t.Error(err)
	}

	expected := []string{
		cpu,
		io,
		memory,
	}
	controllers, err := readControllers(service.path)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(controllers, expected) {
		t.Errorf("unexpected controllers; actual: %v, expected: %v", controllers, expected)
	}

	if err := service.Cleanup(); err != nil {
		t.Error(err)
		return
	}

	if _, err := os.Stat(service.path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected cgroup to not exist; path: %s, err: %v", service.path, err)
		return
	}
}

func TestCleanupWithCgroups(t *testing.T) {
	if !isRoot() {
		t.Skip("must be root to run")
	}

	service, err := NewService()
	if err != nil {
		t.Error(err)
		return
	}

	if _, err := service.CreateCgroup(); err != nil {
		t.Error(err)
	}

	if err := service.Cleanup(); err != nil {
		t.Error(err)
		return
	}

	if _, err := os.Stat(service.path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected cgroup to not exist; path: %s, err: %v", service.path, err)
		return
	}
}

func TestCleanupWithPids(t *testing.T) {
	if !isRoot() {
		t.Skip("must be root to run")
	}

	service, err := NewService()
	if err != nil {
		t.Error(err)
		return
	}

	cgroup, err := service.CreateCgroup()
	if err != nil {
		t.Error(err)
	}

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Error(err)
	}

	if err := service.PlaceInCgroup(*cgroup, cmd.Process.Pid); err != nil {
		t.Error(err)
	}

	if err := service.Cleanup(); err != nil {
		t.Error(err)
	}

	if _, err := os.Stat(service.path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected cgroup to not exist; path: %s, err: %v", service.path, err)
		return
	}
}

func TestCreateCgroup(t *testing.T) {
	if !isRoot() {
		t.Skip("must be root to run")
	}

	service, err := NewService()
	if err != nil {
		t.Error(err)
		return
	}
	// defer func() {
	// 	if err := service.Cleanup(); err != nil {
	// 		t.Error(err)
	// 	}
	// }()

	tests := map[string]struct {
		options []CgroupOption
	}{
		// "no options":              {},
		// "w/ memory limit":         {options: []CgroupOption{WithMemory(1000000000)}},
		// "w/ cpu limit":            {options: []CgroupOption{WithCpus(1.5)}},
		"w/ disk write bps limit": {options: []CgroupOption{WithDiskWriteBps(100000)}},
		"w/ disk read bps limit":  {options: []CgroupOption{WithDiskReadBps(100000)}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cgroup, err := service.CreateCgroup(test.options...)
			if err != nil {
				t.Error(err)
				return
			}

			if _, err := os.Stat(cgroup.path()); err != nil {
				t.Errorf("expected cgroup to exist; path: %s", cgroup.path())
				return
			}
		})
	}
}

func TestPlaceInCgroup(t *testing.T) {
	if !isRoot() {
		t.Skip("must be root to run")
	}

	service, err := NewService()
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		if err := service.Cleanup(); err != nil {
			t.Error(err)
		}
	}()

	cgroup, err := service.CreateCgroup()
	if err != nil {
		t.Error(err)
		return
	}

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Error(err)
		return
	}

	if err := service.PlaceInCgroup(*cgroup, cmd.Process.Pid); err != nil {
		t.Error(err)
		return
	}

	pids, err := cgroup.readPids()
	if err != nil {
		t.Error(err)
		return
	}
	if len(pids) != 1 {
		t.Errorf("unexpected pids; actual: %v, expected: %v", pids, cmd.Process.Pid)
		return
	}
	if pids[0] != cmd.Process.Pid {
		t.Errorf("unexpected pid; actual: %v, expected: %v", pids[0], cmd.Process.Pid)
		return
	}
}

func readControllers(dir string) ([]string, error) {
	fd, err := os.Open(path.Join(dir, cgroupSubtreeControl))
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	scanner := bufio.NewScanner(fd)
	scanner.Split(bufio.ScanWords)

	var controllers []string
	for scanner.Scan() {
		controllers = append(controllers, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return controllers, nil
}

func isRoot() bool {
	return os.Getegid() == 0
}
