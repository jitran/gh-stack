package stack

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const lockFileName = "gh-stack.lock"

// LockTimeout is how long Lock() will wait for the exclusive lock before
// giving up.  This prevents processes (including AI agents) from hanging
// indefinitely when another instance holds the lock.
const LockTimeout = 30 * time.Second

// lockRetryInterval is the sleep between non-blocking lock attempts.
const lockRetryInterval = 100 * time.Millisecond

// FileLock provides an exclusive advisory lock on the stack file to prevent
// concurrent Load-Modify-Save races between multiple gh-stack processes.
type FileLock struct {
	f *os.File
}

// Lock acquires an exclusive lock on the stack file in the given git directory.
// It retries with a non-blocking attempt every 100ms for up to LockTimeout.
// Callers must defer Unlock() to release the lock.
//
// Usage:
//
//	lock, err := stack.Lock(gitDir)
//	if err != nil { ... }
//	defer lock.Unlock()
//	sf, err := stack.Load(gitDir)
//	// ... modify sf ...
//	stack.Save(gitDir, sf)
func Lock(gitDir string) (*FileLock, error) {
	path := filepath.Join(gitDir, lockFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	deadline := time.Now().Add(LockTimeout)
	for {
		err := tryLockFile(f)
		if err == nil {
			return &FileLock{f: f}, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("timed out waiting for stack lock after %s — another gh-stack process may be running", LockTimeout)
		}
		time.Sleep(lockRetryInterval)
	}
}

// Unlock releases the lock.  The lock file is intentionally left on disk to
// avoid a race where another process opens the same path, blocks on flock,
// then wakes up holding a lock on an unlinked inode while a third process
// creates a new file and locks a different inode.
func (l *FileLock) Unlock() {
	if l == nil || l.f == nil {
		return
	}
	unlockFile(l.f)
	l.f.Close()
}
