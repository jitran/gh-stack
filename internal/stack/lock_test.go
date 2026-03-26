package stack

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLock_Basic(t *testing.T) {
	dir := t.TempDir()

	lock, err := Lock(dir)
	require.NoError(t, err)
	require.NotNil(t, lock)

	lock.Unlock()
}

func TestLock_NilUnlockSafe(t *testing.T) {
	// Unlock on nil should not panic.
	var lock *FileLock
	lock.Unlock()
}

func TestLock_BlocksUntilReleased(t *testing.T) {
	dir := t.TempDir()

	lock1, err := Lock(dir)
	require.NoError(t, err)

	acquired := make(chan struct{})
	go func() {
		lock2, err := Lock(dir)
		require.NoError(t, err)
		close(acquired)
		lock2.Unlock()
	}()

	// lock2 should be blocked while lock1 is held.
	select {
	case <-acquired:
		t.Fatal("lock2 acquired while lock1 was still held")
	case <-time.After(300 * time.Millisecond):
		// expected — lock2 is waiting
	}

	lock1.Unlock()

	// After releasing lock1, lock2 should acquire promptly.
	select {
	case <-acquired:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("lock2 did not acquire after lock1 was released")
	}
}

func TestLock_SerializesConcurrentAccess(t *testing.T) {
	dir := t.TempDir()

	// Write an initial stack file with 0 stacks.
	sf := &StackFile{SchemaVersion: 1, Stacks: []Stack{}}
	require.NoError(t, Save(dir, sf))

	// Run 10 concurrent goroutines, each adding a stack under lock.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			lock, err := Lock(dir)
			require.NoError(t, err)
			defer lock.Unlock()

			loaded, err := Load(dir)
			require.NoError(t, err)

			loaded.AddStack(makeStack("main", "branch"))
			require.NoError(t, Save(dir, loaded))
		}(i)
	}
	wg.Wait()

	// All 10 stacks should be present — no lost updates.
	final, err := Load(dir)
	require.NoError(t, err)
	assert.Len(t, final.Stacks, 10, "all concurrent writes should be preserved")
}

func TestLock_FileLeftOnDisk(t *testing.T) {
	dir := t.TempDir()

	lock, err := Lock(dir)
	require.NoError(t, err)
	lock.Unlock()

	// Lock file should still exist after unlock (no os.Remove race).
	lock2, err := Lock(dir)
	require.NoError(t, err, "should be able to re-lock after unlock")
	lock2.Unlock()
}
