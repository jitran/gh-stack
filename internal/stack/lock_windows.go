//go:build windows

package stack

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockFile(f *os.File) error {
	// Lock the first byte exclusively (blocks until available).
	ol := new(windows.Overlapped)
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,    // reserved
		1,    // lock 1 byte
		0,    // high word
		ol,
	)
}

func unlockFile(f *os.File) {
	ol := new(windows.Overlapped)
	_ = windows.UnlockFileEx(
		windows.Handle(f.Fd()),
		0, 1, 0, ol,
	)
}
