package state

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLockStress_Contention tests heavy lock contention
func TestLockStress_Contention(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "lock-stress-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "test.json")

	const (
		numGoroutines = 100
		opsPerRoutine = 50
	)

	var (
		successfulLocks int32
		failedLocks     int32
		wg              sync.WaitGroup
	)

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < opsPerRoutine; j++ {
				lm := NewLockManager(statePath)

				err := lm.Lock(fmt.Sprintf("operation-%d-%d", id, j))
				if err != nil {
					atomic.AddInt32(&failedLocks, 1)
					continue
				}

				atomic.AddInt32(&successfulLocks, 1)

				// Hold lock briefly
				time.Sleep(time.Microsecond * 100)

				if err := lm.Unlock(); err != nil {
					t.Errorf("Failed to unlock: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	totalOps := numGoroutines * opsPerRoutine
	t.Logf("Lock contention test:")
	t.Logf("  Total operations: %d", totalOps)
	t.Logf("  Successful locks: %d", successfulLocks)
	t.Logf("  Failed locks: %d", failedLocks)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.0f ops/sec", float64(totalOps)/duration.Seconds())

	// We expect many failures due to contention, but should have some successes
	if successfulLocks == 0 {
		t.Fatal("No successful locks - something is wrong")
	}
}

// TestLockStress_Sequential tests sequential lock/unlock cycles
func TestLockStress_Sequential(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "lock-seq-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "test.json")

	const iterations = 1000

	start := time.Now()

	for i := 0; i < iterations; i++ {
		lm := NewLockManager(statePath)

		if err := lm.Lock(fmt.Sprintf("op-%d", i)); err != nil {
			t.Fatalf("Iteration %d: failed to lock: %v", i, err)
		}

		if err := lm.Unlock(); err != nil {
			t.Fatalf("Iteration %d: failed to unlock: %v", i, err)
		}
	}

	duration := time.Since(start)
	avgCycle := duration / iterations

	t.Logf("Sequential lock test:")
	t.Logf("  Iterations: %d", iterations)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Average cycle: %v", avgCycle)
}

// TestLockStress_AbandonedLock tests detection of abandoned locks
func TestLockStress_AbandonedLock(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "lock-abandoned-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "test.json")

	// Create an abandoned lock
	lm1 := NewLockManager(statePath)
	if err := lm1.Lock("abandoned"); err != nil {
		t.Fatal(err)
	}
	// Don't unlock - simulate crash

	// Try to acquire lock with another manager
	lm2 := NewLockManager(statePath)
	err = lm2.Lock("new-operation")

	// Should fail because lock exists
	if err == nil {
		t.Fatal("Expected lock acquisition to fail, but it succeeded")
	}

	t.Logf("Correctly detected abandoned lock: %v", err)

	// Force unlock
	if err := lm2.ForceUnlock(); err != nil {
		t.Fatalf("Failed to force unlock: %v", err)
	}

	// Now should succeed
	lm3 := NewLockManager(statePath)
	if err := lm3.Lock("after-force"); err != nil {
		t.Fatalf("Failed to lock after force unlock: %v", err)
	}

	if err := lm3.Unlock(); err != nil {
		t.Fatalf("Failed to unlock: %v", err)
	}

	t.Log("Successfully recovered from abandoned lock")
}

// TestLockStress_RapidAcquireRelease tests rapid lock cycling
func TestLockStress_RapidAcquireRelease(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "lock-rapid-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "test.json")

	const iterations = 10000

	lm := NewLockManager(statePath)
	start := time.Now()

	for i := 0; i < iterations; i++ {
		if err := lm.Lock("rapid"); err != nil {
			t.Fatalf("Iteration %d: lock failed: %v", i, err)
		}

		if err := lm.Unlock(); err != nil {
			t.Fatalf("Iteration %d: unlock failed: %v", i, err)
		}
	}

	duration := time.Since(start)

	t.Logf("Rapid lock cycling:")
	t.Logf("  Iterations: %d", iterations)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Average: %v per cycle", duration/iterations)
	t.Logf("  Rate: %.0f cycles/sec", float64(iterations)/duration.Seconds())
}

// TestLockStress_ManyManagers tests many lock managers competing
func TestLockStress_ManyManagers(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "lock-many-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "test.json")

	const (
		numManagers     = 200
		attemptsPerMgr  = 10
		lockHoldTimeMs  = 5
	)

	var (
		successCount int32
		wg           sync.WaitGroup
	)

	managers := make([]*LockManager, numManagers)
	for i := range managers {
		managers[i] = NewLockManager(statePath)
	}

	start := time.Now()

	for i, lm := range managers {
		wg.Add(1)
		go func(id int, mgr *LockManager) {
			defer wg.Done()

			for j := 0; j < attemptsPerMgr; j++ {
				err := mgr.Lock(fmt.Sprintf("mgr-%d-attempt-%d", id, j))
				if err != nil {
					// Expected - contention
					time.Sleep(time.Millisecond)
					continue
				}

				atomic.AddInt32(&successCount, 1)
				time.Sleep(time.Millisecond * lockHoldTimeMs)

				if err := mgr.Unlock(); err != nil {
					t.Errorf("Manager %d: unlock failed: %v", id, err)
				}

				time.Sleep(time.Millisecond)
			}
		}(i, lm)
	}

	wg.Wait()
	duration := time.Since(start)

	t.Logf("Many managers test:")
	t.Logf("  Managers: %d", numManagers)
	t.Logf("  Attempts each: %d", attemptsPerMgr)
	t.Logf("  Successful locks: %d", successCount)
	t.Logf("  Duration: %v", duration)

	if successCount == 0 {
		t.Fatal("No locks succeeded - deadlock or severe contention")
	}
}

// TestLockStress_StarvationCheck tests for lock starvation
func TestLockStress_StarvationCheck(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "lock-starve-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "test.json")

	const (
		numCompetitors = 20
		duration       = 3 * time.Second
	)

	type stats struct {
		id            int
		successCount  int
		failureCount  int
		totalWaitTime time.Duration
	}

	results := make([]stats, numCompetitors)
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < numCompetitors; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			lm := NewLockManager(statePath)
			st := &results[id]
			st.id = id

			for {
				select {
				case <-stop:
					return
				default:
					waitStart := time.Now()

					err := lm.Lock(fmt.Sprintf("competitor-%d", id))
					waitDuration := time.Since(waitStart)

					if err != nil {
						st.failureCount++
						time.Sleep(time.Millisecond * 10)
						continue
					}

					st.successCount++
					st.totalWaitTime += waitDuration

					// Hold lock briefly
					time.Sleep(time.Millisecond * 5)

					if err := lm.Unlock(); err != nil {
						t.Errorf("Competitor %d: unlock failed: %v", id, err)
					}
				}
			}
		}(i)
	}

	time.Sleep(duration)
	close(stop)
	wg.Wait()

	// Analyze for starvation
	t.Log("Lock starvation analysis:")
	minSuccess := results[0].successCount
	maxSuccess := results[0].successCount

	for _, st := range results {
		if st.successCount < minSuccess {
			minSuccess = st.successCount
		}
		if st.successCount > maxSuccess {
			maxSuccess = st.successCount
		}

		avgWait := time.Duration(0)
		if st.successCount > 0 {
			avgWait = st.totalWaitTime / time.Duration(st.successCount)
		}

		t.Logf("  Competitor %d: success=%d, failures=%d, avg_wait=%v",
			st.id, st.successCount, st.failureCount, avgWait)
	}

	// Check for severe starvation
	fairnessRatio := float64(minSuccess) / float64(maxSuccess)
	t.Logf("  Fairness ratio: %.2f (min=%d, max=%d)", fairnessRatio, minSuccess, maxSuccess)

	if fairnessRatio < 0.1 {
		t.Logf("  WARNING: Potential starvation detected (ratio < 0.1)")
	}
}

// TestLockStress_LockFileCorruption tests handling of corrupted lock files
func TestLockStress_LockFileCorruption(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "lock-corrupt-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testCases := []struct {
		name        string
		lockContent string
		expectError bool
	}{
		{
			name:        "empty lock file",
			lockContent: "",
			expectError: true,
		},
		{
			name:        "invalid json",
			lockContent: "{corrupted",
			expectError: true,
		},
		{
			name:        "valid but incomplete",
			lockContent: `{"id": "test"}`,
			expectError: false, // Should still fail to unlock since ID won't match
		},
		{
			name:        "null bytes",
			lockContent: string([]byte{0, 0, 0}),
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			statePath := filepath.Join(tmpDir, tc.name, "state.json")
			lockPath := statePath + ".lock"

			if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
				t.Fatal(err)
			}

			// Write corrupted lock file
			if err := ioutil.WriteFile(lockPath, []byte(tc.lockContent), 0644); err != nil {
				t.Fatal(err)
			}

			// Try to acquire lock (should fail due to existing lock)
			lm := NewLockManager(statePath)
			err := lm.Lock("test")

			if err == nil {
				t.Log("Lock succeeded despite existing lock file")
				lm.Unlock()
			} else {
				t.Logf("Lock correctly failed: %v", err)
			}
		})
	}
}

// BenchmarkLock_AcquireRelease benchmarks lock acquire/release
func BenchmarkLock_AcquireRelease(b *testing.B) {
	tmpDir, err := ioutil.TempDir("", "bench-lock-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "bench.json")
	lm := NewLockManager(statePath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := lm.Lock("bench"); err != nil {
			b.Fatal(err)
		}
		if err := lm.Unlock(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLock_Contention benchmarks lock contention
func BenchmarkLock_Contention(b *testing.B) {
	tmpDir, err := ioutil.TempDir("", "bench-contend-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "bench.json")

	b.RunParallel(func(pb *testing.PB) {
		lm := NewLockManager(statePath)
		for pb.Next() {
			err := lm.Lock("bench")
			if err == nil {
				lm.Unlock()
			}
		}
	})
}
