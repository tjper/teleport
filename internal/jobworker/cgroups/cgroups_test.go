package cgroups

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
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

	if _, err := os.Stat(cgroup.path()); err != nil {
		t.Errorf("expected cgroup to exist; path: %s", cgroup.path())
		return
	}
}

func isRoot() bool {
	return os.Getegid() == 0
}
