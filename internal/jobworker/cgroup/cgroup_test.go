package cgroup

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"reflect"
	"strconv"
	"testing"

	"github.com/tjper/teleport/internal/device"
)

func TestServiceSetupAndCleanup(t *testing.T) {
	if !isRoot() {
		t.Skip("must be root to run")
	}

	dir := t.TempDir()
	service, err := NewService(WithMountPath(dir))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if _, err := os.Stat(service.path); err != nil {
		t.Fatalf("stat service cgroup; path: %s, error: %s", service.path, err)
	}

	expected := []string{
		cpu,
		io,
		memory,
	}
	controllers, err := readControllers(service.path)
	if err != nil {
		t.Fatalf("read service controllers; path: %s, error: %s", service.path, err)
	}

	if !reflect.DeepEqual(controllers, expected) {
		t.Fatalf("unexpected controllers; actual: %v, expected: %v", controllers, expected)
	}

	if err := service.Cleanup(); err != nil {
		t.Fatalf("service cleanup; error: %s", err)
	}

	if _, err := os.Stat(service.path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected cgroup to not exist; path: %s, error: %v", service.path, err)
	}
}

func TestCleanupWithCgroups(t *testing.T) {
	if !isRoot() {
		t.Skip("must be root to run")
	}

	dir := t.TempDir()
	service, err := NewService(WithMountPath(dir))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.CreateCgroup(); err != nil {
		t.Fatal(err)
	}

	if err := service.Cleanup(); err != nil {
		t.Fatalf("service cleanup; error: %s", err)
	}

	if _, err := os.Stat(service.path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected cgroup to not exist; path: %s, err: %v", service.path, err)
	}
}

func TestCleanupWithPids(t *testing.T) {
	if !isRoot() {
		t.Skip("must be root to run")
	}

	dir := t.TempDir()
	service, err := NewService(WithMountPath(dir))
	if err != nil {
		t.Fatal(err)
	}

	cgroup, err := service.CreateCgroup()
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("exec sleep 30: %s", err)
	}

	if err := service.PlaceInCgroup(*cgroup, cmd.Process.Pid); err != nil {
		t.Fatalf("place in cgroup; pid: %d, error: %s", cmd.Process.Pid, err)
	}

	if err := service.Cleanup(); err != nil {
		t.Fatalf("service cleanup; error: %s", err)
	}

	if _, err := os.Stat(service.path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected cgroup to not exist; path: %s, err: %v", service.path, err)
		return
	}
}

func TestCreateCgroup(t *testing.T) {
	if !isRoot() {
		t.Skip("must be root to run")
	}

	dir := t.TempDir()
	service, err := NewService(WithMountPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := service.Cleanup(); err != nil {
			t.Fatal(err)
		}
	}()

	tests := map[string]struct {
		options []CgroupOption
	}{
		"no options":              {},
		"w/ memory limit":         {options: []CgroupOption{WithMemory(1000000000)}},
		"w/ cpu limit":            {options: []CgroupOption{WithCpus(1.5)}},
		"w/ disk write bps limit": {options: []CgroupOption{WithDiskWriteBps(100000)}},
		"w/ disk read bps limit":  {options: []CgroupOption{WithDiskReadBps(100000)}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cgroup, err := service.CreateCgroup(test.options...)
			if err != nil {
				t.Fatalf("create cgroup error: %s", err)
			}

			if _, err := os.Stat(cgroup.path); err != nil {
				t.Fatalf("expected cgroup to exist; path: %s", cgroup.path)
			}
		})
	}
}

func TestPlaceInCgroup(t *testing.T) {
	if !isRoot() {
		t.Skip("must be root to run")
	}

	dir := t.TempDir()
	service, err := NewService(WithMountPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := service.Cleanup(); err != nil {
			t.Fatal(err)
		}
	}()

	cgroup, err := service.CreateCgroup()
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("exec sleep 30: %s", err)
	}

	if err := service.PlaceInCgroup(*cgroup, cmd.Process.Pid); err != nil {
		t.Fatalf("place in cgroup; pid: %d, error: %s", cmd.Process.Pid, err)
	}

	pids, err := readPids(cgroup.path)
	if err != nil {
		t.Fatal(err)
		return
	}
	if len(pids) != 1 {
		t.Fatalf("unexpected pids; actual: %v, expected: %v", pids, cmd.Process.Pid)
	}
	if pids[0] != cmd.Process.Pid {
		t.Fatalf("unexpected pid; actual: %v, expected: %v", pids[0], cmd.Process.Pid)
	}
}

func TestControllers(t *testing.T) {
	dir := t.TempDir()
	cgroup := Cgroup{path: dir}

	type expected struct {
		enabled string
		values  string
	}
	tests := map[string]struct {
		file       string
		controller controller
		exp        expected
	}{
		"memory": {
			file:       "memory.high",
			controller: newMemoryController(cgroup, 1024),
			exp: expected{
				enabled: "+memory\n",
				values:  "1024",
			},
		},
		"cpu": {
			file:       "cpu.max",
			controller: newCPUController(cgroup, 1.5),
			exp: expected{
				enabled: "+cpu\n",
				values:  "150000 100000",
			},
		},
		"disk rbps": {
			file:       "io.max",
			controller: newDiskReadBpsController(cgroup, 2048),
			exp: expected{
				enabled: "+io\n",
				values:  ioMaxValue(t, "rbps", "2048"),
			},
		},
		"disk wbps": {
			file:       "io.max",
			controller: newDiskWriteBpsController(cgroup, 4096),
			exp: expected{
				enabled: "+io\n",
				values:  ioMaxValue(t, "wbps", "4096"),
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if err := test.controller.enable(); err != nil {
				t.Fatalf("enable controller; error: %s", err)
			}
			if err := test.controller.apply(); err != nil {
				t.Fatalf("apply controller; error: %s", err)
			}

			b, err := os.ReadFile(path.Join(dir, cgroupSubtreeControl))
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != test.exp.enabled {
				t.Fatalf("controllers unexpected; actual: %s, expected: %s", b, test.exp.enabled)
			}

			b, err = os.ReadFile(path.Join(dir, test.file))
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != test.exp.values {
				t.Fatalf("control values unexpected; actual: %s, expected: %s", b, test.exp.values)
			}
		})
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

func readPids(dir string) ([]int, error) {
	file := path.Join(dir, cgroupProcs)
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	var pids []int
	procs := bufio.NewScanner(fd)
	for procs.Scan() {
		pid, err := strconv.Atoi(procs.Text())
		if err != nil {
			return nil, err
		}
		pids = append(pids, pid)
	}
	if procs.Err() != nil {
		return nil, err
	}

	return pids, nil
}

func ioMaxValue(t *testing.T, key, value string) string {
	minors, err := device.ReadDeviceMinors(diskDevices, diskPhysicalMinors)
	if err != nil {
		t.Fatal(t)
	}

	var max uint32
	for _, minor := range minors {
		if minor > max {
			max = minor
		}
	}
	return fmt.Sprintf("%d:%d %s=%s", diskDevices, max, key, value)
}

func isRoot() bool {
	return os.Getegid() == 0
}
