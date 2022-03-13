// Package watch provides types for watching file modifications.
package watch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	ierrors "github.com/tjper/teleport/internal/errors"
)

// ErrNotFile indicates that non file path was specified for the ModWatcher.
var ErrNotFile = errors.New("not file")

// NewModWatcher creates a ModWatcher instance.
func NewModWatcher(path string) *ModWatcher {
	return &ModWatcher{
		mutex:     new(sync.RWMutex),
		path:      filepath.Clean(path),
		listeners: make(map[uuid.UUID]chan struct{}),
	}
}

// Modwatcher watches a single file for modifications.
type ModWatcher struct {
	mutex *sync.RWMutex

	path      string
	modTime   time.Time
	listeners map[uuid.UUID]chan struct{}
}

// Watch checks the ModWatcher path periodically to see if any modifications
// have occurred since the last check. The tick argument determines the
// interval between checks. Watch is blocking and will return if the ctx is
// canceled or an error occurs.
func (w *ModWatcher) Watch(ctx context.Context, tick time.Duration) error {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ierrors.Wrap(ctx.Err())
		case <-ticker.C:
			info, err := os.Stat(w.path)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return ierrors.Wrap(err)
			}
			if info.IsDir() {
				return fmt.Errorf("%w; path: %s", ErrNotFile, w.path)
			}

			if w.modTime.Equal(info.ModTime()) {
				continue
			}
			w.modTime = info.ModTime()

			w.broadcast()
		}
	}
}

// WaitUntil blocks until the ModWatcher detects a modification or the ctx
// is canceled.
func (w *ModWatcher) WaitUntil(ctx context.Context) error {
	w.mutex.Lock()
retry:
	id := uuid.New()
	if _, ok := w.listeners[id]; ok {
		goto retry
	}

	modification := make(chan struct{}, 1)
	w.listeners[id] = modification
	w.mutex.Unlock()

	defer func() {
		w.mutex.Lock()
		delete(w.listeners, id)
		w.mutex.Unlock()
	}()

	select {
	case <-ctx.Done():
		return ierrors.Wrap(ctx.Err())
	case <-modification:
		return nil
	}
}

// broadcast publishes to all ModWatcher listeners that a modification has
// occurred.
func (w ModWatcher) broadcast() {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	for _, listener := range w.listeners {
		listener <- struct{}{}
	}
}
