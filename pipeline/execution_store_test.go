package pipeline

import (
	"math/rand"
	"fmt"
	"sync"
	"testing"
	"time"
)

type mockTimeProvider struct {
    currentTime time.Time
    mutex       sync.Mutex
}

func (mtp *mockTimeProvider) Now() time.Time {
    mtp.mutex.Lock()
    defer mtp.mutex.Unlock()
    return mtp.currentTime
}

func (mtp *mockTimeProvider) Add(d time.Duration) {
    mtp.mutex.Lock()
    mtp.currentTime = mtp.currentTime.Add(d)
    mtp.mutex.Unlock()
}


func TestConcurrentOperations(t *testing.T) {
    startTime := time.Now()
    mtp := &mockTimeProvider{currentTime: startTime}
    timeProvider = mtp
    defer func() { timeProvider = &realTimeProvider{} }()

    threshold := 5 * time.Minute
    cleanupInterval := 100 * time.Millisecond // More frequent cleanup for testing

    // Start the cleanup process
    StartExecutionStoreCleanup(threshold, cleanupInterval)
    defer StopExecutionStoreCleanup()

    var wg sync.WaitGroup
    for i := 0; i < 1000; i++ { // Increase the number of goroutines
        wg.Add(1)
        go func() {
            defer wg.Done()
            addRandomExecution(mtp.Now())
        }()
    }

    // Simulate time passing and more executions being added
    for i := 0; i < 10; i++ {
        mtp.Add(cleanupInterval)
        time.Sleep(10 * time.Millisecond) // Allow cleanup goroutine to run

        for j := 0; j < 100; j++ { // Add more executions each iteration
            wg.Add(1)
            go func() {
                defer wg.Done()
                addRandomExecution(mtp.Now())
            }()
        }
    }

    wg.Wait()

    // Final cleanup
    mtp.Add(threshold + time.Second)
    performCleanup(threshold)

    // Verify that all expired executions have been cleaned up
    ExecutionStore.RLock()
    defer ExecutionStore.RUnlock()
    for _, exec := range ExecutionStore.Executions {
        completedAt, _ := time.Parse(time.RFC3339, exec.CompletedAt)
        if mtp.Now().Sub(completedAt) > threshold {
            t.Errorf("Found expired execution that should have been cleaned up: %v", exec)
        }
    }
}

func addRandomExecution(now time.Time) {
    id := fmt.Sprintf("exec_%d", rand.Int())
    completedAt := now.Add(-time.Duration(rand.Intn(600)) * time.Second)
    result := &ExecutionResult{
        ExecutionID: id,
        Status:      StatusCompleted,
        CompletedAt: completedAt.Format(time.RFC3339),
    }
    AddExecution(id, result)
}
