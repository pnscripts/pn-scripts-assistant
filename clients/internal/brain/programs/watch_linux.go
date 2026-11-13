//go:build linux

package programs

import (
	"context"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// addWatch is inotify_add_watch; a test replaces it to be refused.
var addWatch = unix.InotifyAddWatch

// watchMask is everything that changes what a folder holds, and the folder
// itself going away.
const watchMask = unix.IN_CREATE | unix.IN_DELETE | unix.IN_MOVED_FROM | unix.IN_MOVED_TO |
	unix.IN_CLOSE_WRITE | unix.IN_ATTRIB | unix.IN_DELETE_SELF | unix.IN_MOVE_SELF | unix.IN_ONLYDIR

// inotify is one inotify instance. Non-blocking, so the runtime's poller
// waits on it and closing the file ends a read in progress.
type inotify struct {
	fd   int
	file *os.File
}

func newEvents() (events, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}

	return &inotify{fd: fd, file: os.NewFile(uintptr(fd), "inotify")}, nil
}

func (in *inotify) add(dir string) (int, error) { return addWatch(in.fd, dir, watchMask) }

func (in *inotify) remove(wd int) { unix.InotifyRmWatch(in.fd, uint32(wd)) }

func (in *inotify) close() { in.file.Close() }

// readEvents hands every event to w until the instance is closed.
func readEvents(ctx context.Context, w *Watching, ev events) {
	in := ev.(*inotify)
	buf := make([]byte, 64<<10)

	for {
		n, err := in.file.Read(buf)
		if err != nil || ctx.Err() != nil {
			return
		}

		for off := 0; off+unix.SizeofInotifyEvent <= n; {
			e := (*unix.InotifyEvent)(unsafe.Pointer(&buf[off]))
			end := off + unix.SizeofInotifyEvent + int(e.Len)

			if end > n {
				break
			}

			name := strings.TrimRight(string(buf[off+unix.SizeofInotifyEvent:end]), "\x00")
			gone := e.Mask&(unix.IN_DELETE_SELF|unix.IN_MOVE_SELF|unix.IN_IGNORED) != 0

			w.event(int(e.Wd), name, gone, e.Mask&unix.IN_Q_OVERFLOW != 0)

			off = end
		}
	}
}
