package stack

import (
	"fmt"
	"os"
	"path/filepath"
)

const lockFileName = "gh-stack.lock"

// FileLock provides an exclusive advisory lock on the stack file to prevent
// concurrent Load-Modify-Save races between multiple gh-stack processes.
type FileLock struct {
	f    *os.File
	path string
}

// Lock acquires an exclusive lock on the stack file in the given git directory.
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

	if err := lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquiring lock: %w", err)
	}

	return &FileLock{f: f, path: path}, nil
}

// Unlock releases the lock and removes the lock file.
func (l *FileLock) Unlock() {
	if l == nil || l.f == nil {
		return
	}
	unlockFile(l.f)
	l.f.Close()
	os.Remove(l.path)
}
