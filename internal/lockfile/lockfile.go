//go:build linux || darwin

// Package lockfile provides advisory process locks for Corum's supported
// POSIX release targets.
package lockfile

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// ErrLocked means another process currently owns a non-blocking lock.
var ErrLocked = errors.New("resource is locked by another Corum process")

// Lock is an exclusive advisory lock held by an open file descriptor.
type Lock struct {
	file *os.File
}

// Acquire waits for an exclusive lock. The parent directory must already exist.
func Acquire(path string) (*Lock, error) {
	return acquire(path, false)
}

// TryAcquire obtains an exclusive lock without waiting.
func TryAcquire(path string) (*Lock, error) {
	return acquire(path, true)
}

func acquire(path string, nonblocking bool) (*Lock, error) {
	if path == "" {
		return nil, fmt.Errorf("lock path is required")
	}
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	operation := unix.LOCK_EX
	if nonblocking {
		operation |= unix.LOCK_NB
	}
	if err := unix.Flock(fd, operation); err != nil {
		_ = file.Close()
		if nonblocking && (errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	return &Lock{file: file}, nil
}

// Close releases the lock.
func (lock *Lock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	file := lock.file
	lock.file = nil
	unlockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN)
	closeErr := file.Close()
	return errors.Join(unlockErr, closeErr)
}
