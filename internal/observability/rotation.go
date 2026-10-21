package observability

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// Reopen under the shared file lock on every write, so concurrent CLI processes
// cannot retain a descriptor to a rotated inode or interleave JSON records.
type rotatingWriter struct {
	path    string
	max     int64
	backups int
	mu      sync.Mutex
}

func (w *rotatingWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if int64(len(data)) > w.max {
		return 0, errors.New("diagnostic record exceeds log bound")
	}
	fd, e := unix.Open(w.path+".lock", unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return 0, e
	}
	lock := os.NewFile(uintptr(fd), "diagnostic lock")
	defer lock.Close()
	if e = unix.Flock(fd, unix.LOCK_EX); e != nil {
		return 0, e
	}
	if info, e := os.Lstat(w.path); e == nil {
		if !info.Mode().IsRegular() {
			return 0, errors.New("invalid diagnostic file")
		}
		if info.Size()+int64(len(data)) > w.max {
			for n := w.backups; n > 0; n-- {
				source := w.path
				if n > 1 {
					source += "." + strconv.Itoa(n-1)
				}
				if e = os.Rename(source, w.path+"."+strconv.Itoa(n)); e != nil && !os.IsNotExist(e) {
					return 0, e
				}
			}
		}
	} else if !os.IsNotExist(e) {
		return 0, e
	}
	fd, e = unix.Open(w.path, unix.O_CREAT|unix.O_WRONLY|unix.O_APPEND|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return 0, e
	}
	f := os.NewFile(uintptr(fd), "diagnostic file")
	defer f.Close()
	return f.Write(data)
}
func (w *rotatingWriter) Close() error { return nil }
func newWriter(dir, name string, max int64, backups int) (*rotatingWriter, error) {
	if max < 1024 || backups < 1 || backups > 10 {
		return nil, errors.New("invalid diagnostic bounds")
	}
	info, e := os.Lstat(dir)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("invalid diagnostic directory")
	}
	w := &rotatingWriter{path: filepath.Join(dir, name), max: max, backups: backups}
	if _, e = w.Write(nil); e != nil {
		return nil, e
	}
	return w, nil
}
