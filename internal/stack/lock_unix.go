//go:build !windows

package stack

import (
	"os"
	"syscall"
)

// tryLockFile attempts a non-blocking exclusive lock.
// Returns nil on success, or an error if the lock is held by another process.
func tryLockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func unlockFile(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
