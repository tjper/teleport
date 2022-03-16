package cgroup

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"reflect"
	"strconv"
	"testing"
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

func isRoot() bool {
	return os.Getegid() == 0
}
