package stack

import (
	"sync"
	"testing"

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
