package indexer

import (
	"sync"
	"testing"
	"time"
)

func TestSyncManagerSerializesLifecycleTransitions(t *testing.T) {
	sm := NewSyncManager(0, 0, nil)
	node := "http://127.0.0.1:1"
	sm.StartSync(node)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		sm.StartSync(node)
	}()
	go func() {
		defer wg.Done()
		sm.StopSync()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent sync lifecycle transition did not complete")
	}

	// Ensure the test leaves no newly-started lifecycle active.
	sm.StopSync()
}

func TestSyncManagerDatabaseRetentionDefaultsToWipe(t *testing.T) {
	sm := NewSyncManager(0, 0, nil)
	if !sm.shouldWipeDB() {
		t.Fatal("new SyncManager should preserve the existing wipe-on-start default")
	}
	sm.SetPreserveDB(true)
	if sm.shouldWipeDB() {
		t.Fatal("--keep-db mode should preserve the existing database")
	}
}
