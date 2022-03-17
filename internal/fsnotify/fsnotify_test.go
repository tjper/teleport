package fsnotify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddRemoveWatch(t *testing.T) {
	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}()

	dir := t.TempDir()
	if _, err := w.AddWatch(dir); err != nil {
		t.Fatalf("expected to be able to add watch; error: %v", err)
	}
	if err := w.RemoveWatch(dir); err != nil {
		t.Fatalf("expected to be able to remove watch; error: %v", err)
	}

	go func() {
		for event := range w.Events {
			t.Logf("event: %v", event)
		}
	}()
}

func TestEvents(t *testing.T) {
	tests := map[string]struct {
		file   string
		do     func(*testing.T, string)
		events []Op
	}{
		"create": {
			file: "create.txt",
			do: func(t *testing.T, file string) {
				if err := os.WriteFile(file, []byte("create"), 0644); err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
			},
			events: []Op{
				Create | Rename,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			w, err := NewWatcher()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer func() {
				if err := w.Close(); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}()

			dir := t.TempDir()
			file := filepath.Join(dir, test.file)

			if err := os.WriteFile(file, nil, 0644); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if _, err := w.AddWatch(file); err != nil {
				t.Fatalf("expected to be able to add watch; error: %v", err)
			}
			defer func() {
				if err := w.RemoveWatch(file); err != nil {
					t.Fatalf("expected to be able to remove watch; error: %v", err)
				}
			}()

			test.do(t, file)

			for event := range w.Events {
				if len(test.events) == 0 {
					t.Fatalf("unexpected event: %v", event)
				}

				expected := test.events[0]
				if event.Op.String() != expected.String() {
					t.Fatalf("unexpected event; actual: %v, expected: %v", event, expected)
				}

				test.events = test.events[1:]
				if len(test.events) == 0 {
					return
				}
			}
		})
	}
}
