//go:build windows

package programs

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

/*
 * Events on Windows: a folder is watched with ReadDirectoryChangesW
 * (overlapped, so a stop never waits on the kernel), a registry key with
 * RegNotifyChangeKeyValue. Each watch is one goroutine blocked in
 * WaitForMultipleObjects on its event and its stop event: an idle
 * computer costs nothing.
 */

// addWatch is asked before a watch is set; a test replaces it to be
// refused, as Linux's inotify_add_watch can be.
var addWatch = func(int, string, uint32) (int, error) { return 0, nil }

// folderChanges is everything that changes what a folder holds.
const folderChanges = windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME |
	windows.FILE_NOTIFY_CHANGE_LAST_WRITE | windows.FILE_NOTIFY_CHANGE_SIZE

type winEvents struct {
	w *Watching

	mu     sync.Mutex
	next   int
	stops  map[int]windows.Handle
	closed bool
	wg     sync.WaitGroup
}

// dirWatch is on the heap and stays there while the kernel writes into
// it: the overlapped structure and the buffer must not move.
type dirWatch struct {
	ov  windows.Overlapped
	buf [64 << 10]byte
}

func newEvents(w *Watching) (events, error) {
	return &winEvents{w: w, stops: map[int]windows.Handle{}}, nil
}

func (e *winEvents) add(t Target) (int, error) {
	if _, err := addWatch(0, t.Dir, 0); err != nil {
		return -1, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return -1, errors.New("the watch has stopped")
	}

	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return -1, err
	}

	wd := e.next

	if t.Registry {
		err = e.watchKey(wd, t, stop)
	} else {
		err = e.watchFolder(wd, t, stop)
	}

	if err != nil {
		windows.CloseHandle(stop)

		return -1, err
	}

	e.next++
	e.stops[wd] = stop

	return wd, nil
}

// remove stops one watch without waiting for it: it may be the watch
// itself saying its folder went away.
func (e *winEvents) remove(wd int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if stop, ok := e.stops[wd]; ok {
		windows.SetEvent(stop)
		delete(e.stops, wd)
	}
}

func (e *winEvents) close() {
	e.mu.Lock()
	e.closed = true

	for wd, stop := range e.stops {
		windows.SetEvent(stop)
		delete(e.stops, wd)
	}
	e.mu.Unlock()

	e.wg.Wait()
}

// readEvents: each watch reports by itself; nothing to read here.
func readEvents(ctx context.Context, _ *Watching, _ events) { <-ctx.Done() }

func (e *winEvents) watchFolder(wd int, t Target, stop windows.Handle) error {
	name, err := windows.UTF16PtrFromString(t.Dir)
	if err != nil {
		return err
	}

	h, err := windows.CreateFile(name, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return err
	}

	ready, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(h)

		return err
	}

	d := &dirWatch{}
	d.ov.HEvent = ready

	// The first read is asked here, so a change made right after Watch
	// returns is not missed.
	if err := windows.ReadDirectoryChanges(h, &d.buf[0], uint32(len(d.buf)), t.Tree, folderChanges, nil, &d.ov, 0); err != nil {
		windows.CloseHandle(ready)
		windows.CloseHandle(h)

		return err
	}

	e.wg.Add(1)

	go func() {
		defer e.wg.Done()
		defer windows.CloseHandle(stop)
		defer windows.CloseHandle(ready)
		defer windows.CloseHandle(h)

		failures := 0

		for {
			r, err := windows.WaitForMultipleObjects([]windows.Handle{ready, stop}, false, windows.INFINITE)
			if err != nil || r != windows.WAIT_OBJECT_0 {
				windows.CancelIoEx(h, &d.ov)

				var n uint32
				windows.GetOverlappedResult(h, &d.ov, &n, true)

				return
			}

			var n uint32

			err = windows.GetOverlappedResult(h, &d.ov, &n, false)

			switch {
			case err == nil && n > 0:
				failures = 0

				for _, name := range changedNames(d.buf[:n]) {
					e.w.event(wd, filepath.Base(name), false, false)
				}
			case !folderThere(t.Dir):
				e.w.event(wd, "", true, false)

				return
			default:
				// Too many changes for the buffer (n == 0): look at all
				// of it. A folder that keeps failing is polled instead.
				failures++
				if failures > 3 {
					e.w.event(wd, "", true, false)

					return
				}

				e.w.event(wd, "", false, true)
			}

			windows.ResetEvent(ready)

			if err := windows.ReadDirectoryChanges(h, &d.buf[0], uint32(len(d.buf)), t.Tree, folderChanges, nil, &d.ov, 0); err != nil {
				e.w.event(wd, "", true, false)

				return
			}
		}
	}()

	return nil
}

func folderThere(dir string) bool {
	st, err := os.Stat(dir)

	return err == nil && st.IsDir()
}

// changedNames reads FILE_NOTIFY_INFORMATION records; every offset is
// checked against what the kernel wrote.
func changedNames(b []byte) []string {
	var out []string

	for at := 0; at+12 <= len(b); {
		next := int(binary.LittleEndian.Uint32(b[at:]))
		size := int(binary.LittleEndian.Uint32(b[at+8:]))

		if at+12+size <= len(b) && size >= 0 {
			u := make([]uint16, size/2)
			for i := range u {
				u[i] = binary.LittleEndian.Uint16(b[at+12+2*i:])
			}

			out = append(out, string(utf16.Decode(u)))
		}

		if next <= 0 {
			break
		}

		at += next
	}

	return out
}

func (e *winEvents) watchKey(wd int, t Target, stop windows.Handle) error {
	k, err := openKey(t.Dir, registry.NOTIFY|registry.QUERY_VALUE)
	if err != nil {
		return err
	}

	changed, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		k.Close()

		return err
	}

	// RegNotifyChangeKeyValue belongs to the thread that asked: it ends
	// when that thread does. The goroutine keeps its thread and asks
	// there; add waits until the first ask is made.
	asked := make(chan error, 1)

	e.wg.Add(1)

	go func() {
		defer e.wg.Done()

		runtime.LockOSThread()
		// Never unlocked: the thread ends with the goroutine, and with it
		// any notification still registered.

		defer windows.CloseHandle(changed)
		defer k.Close()

		const filter = windows.REG_NOTIFY_CHANGE_NAME | windows.REG_NOTIFY_CHANGE_LAST_SET

		notify := func() error {
			return windows.RegNotifyChangeKeyValue(windows.Handle(k), t.Tree, filter, changed, true)
		}

		// Refused: add says so and closes stop itself.
		if err := notify(); err != nil {
			asked <- err

			return
		}

		defer windows.CloseHandle(stop)

		asked <- nil

		for {
			r, err := windows.WaitForMultipleObjects([]windows.Handle{changed, stop}, false, windows.INFINITE)
			if err != nil || r != windows.WAIT_OBJECT_0 {
				return
			}

			if _, ok := keyStamp(t.Dir); !ok {
				e.w.event(wd, "", true, false)

				return
			}

			if err := notify(); err != nil {
				e.w.event(wd, "", true, false)

				return
			}

			e.w.event(wd, "", false, false)
		}
	}()

	return <-asked
}
