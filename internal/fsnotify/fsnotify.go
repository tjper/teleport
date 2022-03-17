// Package fsnotify provides an API for watching for filesystem events.
package fsnotify

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"unsafe"

	"github.com/tjper/teleport/internal/log"

	"golang.org/x/sys/unix"
)

// logger is an object for logging package event to stdout.
var logger = log.New(os.Stdout, "fsnotify")

var (
	// ErrInvalidFD indicates the Watcher was unable to initialize.
	ErrInvalidFD = errors.New("invalid file descriptor")
	// ErrWatchExists indicates the path specifed is already being watched.
	ErrWatchExists = errors.New("path is already being watched")
	// ErrWatchDNE indicates the path specified is not being watched.
	ErrWatchDNE = errors.New("path is not being watched")
)

// NewWatcher creates a Watcher instance.
func NewWatcher() (*Watcher, error) {
	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("init inotify fd for watcher; error: %w", err)
	}

	file := os.NewFile(uintptr(fd), "/proc/self/fd/3")
	if file == nil {
		unix.Close(fd)
		return nil, fmt.Errorf("watcher file descriptor; error: %w", ErrInvalidFD)
	}

	w := &Watcher{
		mutex:   new(sync.Mutex),
		watches: make(map[string]int),
		paths:   make(map[int]string),
		Events:  make(chan Event),
		done:    make(chan struct{}),
		fd:      fd,
		file:    file,
		closed:  make(chan struct{}),
	}

	go w.readEvents()
	return w, nil
}

// Watcher utilizes the inotify API to observe and publish events related
// watched filesystem entities.
type Watcher struct {
	mutex   *sync.Mutex
	watches map[string]int
	paths   map[int]string
	Events  chan Event

	fd   int
	file *os.File

	done   chan struct{}
	closed chan struct{}
}

// AddWatch instructs the Watcher to begin watching the specified path. The
// first return value is watch descriptor unique to this path. If the path is
// being watched, the ErrWatchExists error will be returned.
func (w *Watcher) AddWatch(path string) (int, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	wd, ok := w.watches[path]
	if ok {
		return wd, ErrWatchExists
	}

	wd, err := unix.InotifyAddWatch(w.fd, path, unix.IN_ALL_EVENTS)
	if err != nil {
		return 0, fmt.Errorf("add watch; error: %w", err)
	}

	w.watches[path] = wd
	w.paths[wd] = path

	return wd, nil
}

// RemoveWatch instructs the Watcher to stop watching the specified path. If
// the path is not being watched, the ErrWatchDNE error will be returned.
func (w *Watcher) RemoveWatch(path string) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	wd, ok := w.watches[path]
	if !ok {
		return ErrWatchDNE
	}

	// On success, inotify_rm_watch() returns zero.  On error, -1 is returned
	// and errno is set to  indicate  the cause of the error.
	success, err := unix.InotifyRmWatch(w.fd, uint32(wd))
	if success == -1 {
		return fmt.Errorf("remove watch; error: %w", err)
	}

	delete(w.watches, path)
	delete(w.paths, wd)

	return nil
}

func (w *Watcher) Close() error {
	if w.isDone() {
		return nil
	}

	close(w.done)

	<-w.closed
	return nil
}

// isDone indicates if the watcher has intitiated closing.
func (w *Watcher) isDone() bool {
	select {
	case <-w.done:
		return true
	default:
		return false
	}
}

// readEvents reads inotify events from the Watcher's inotifiy file descriptor
// and publishes them on the Watcher.Events channel.
func (w *Watcher) readEvents() {
	defer close(w.closed)
	defer close(w.Events)

	go func() {
		<-w.done
		if err := w.file.Close(); err != nil {
			logger.Warnf("close watcher; error: %s", err)
		}
	}()

	b := make([]byte, unix.SizeofInotifyEvent)
	for {
		if w.isDone() {
			return
		}

		n, err := io.ReadFull(w.file, b)
		if errors.Is(err, io.ErrUnexpectedEOF) {
			logger.Warnf("inotify event not fully read; size: %d, error: %s", n, err)
			continue
		}
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			logger.Warnf("inotify event read; error: %s", err)
			continue
		}

		raw := (*unix.InotifyEvent)(unsafe.Pointer(&b))
		mask := raw.Mask

		// IN_DELETE_SELF occurs when the file/directory being watched is removed.
		// This should result in cleaning up the maps, otherwise we are no longer
		// in sync with the inotify kernel state.
		w.mutex.Lock()
		path, ok := w.paths[int(raw.Wd)]

		if ok && mask&unix.IN_DELETE_SELF == unix.IN_DELETE_SELF {
			delete(w.paths, int(raw.Wd))
			delete(w.watches, path)
		}
		w.mutex.Unlock()

		select {
		case <-w.done:
			return
		case w.Events <- newEvent(int(raw.Wd), mask, path):
		}
	}
}

func newEvent(wd int, mask uint32, path string) Event {
	e := Event{Wd: wd, Path: path}
	if mask&unix.IN_CREATE == unix.IN_CREATE {
		e.Op |= Create
	}
	if mask&unix.IN_MODIFY == unix.IN_MODIFY {
		e.Op |= Write
	}
	return e
}

type Event struct {
	Op   Op
	Wd   int
	Path string
}

type Op int

const (
	Create Op = 1 << iota
	Write
)

func (op Op) String() string {
	var buffer bytes.Buffer

	if op&Create == Create {
		buffer.WriteString("|CREATE")
	}
	if op&Write == Write {
		buffer.WriteString("|WRITE")
	}
	if buffer.Len() == 0 {
		return ""
	}
	return buffer.String()[1:] // strip leading pipe
}
