package pipeline

import (
	"log"
	"sync"
	"time"
)

type ExecutionStatus string

const (
    StatusStarted   ExecutionStatus = "started"
    StatusCompleted ExecutionStatus = "completed"
    StatusFailed    ExecutionStatus = "failed"
)

type ExecutionResult struct {
    PipelineID    string                 `json:"pipeline_id"`
    ExecutionID   string                 `json:"execution_id"`
    Status        ExecutionStatus        `json:"status"`
    StartTime     int64                  `json:"start_time"`
    EndTime       int64                  `json:"end_time,omitempty"`
    Results       map[string]interface{} `json:"results,omitempty"`
    ErrorMessage  string                 `json:"error_message,omitempty"`
    UserInput     string                 `json:"user_input,omitempty"`
    SubmittedAt   string                 `json:"submitted_at"`
    CompletedAt   string                 `json:"completed_at,omitempty"`
}

// StartExecutionStoreCleanup starts a goroutine that periodically cleans up old execution results.
// - threshold: Duration after which execution results are considered expired.
// - cleanupInterval: How often the cleanup process runs.

var (
    ExecutionStore = struct {
        sync.RWMutex
        Executions map[string]*ExecutionResult
    }{
        Executions: make(map[string]*ExecutionResult),
    }
    cleanupTicker *time.Ticker
    stopCleanup   chan struct{}
)

func StartExecutionStoreCleanup(threshold time.Duration, cleanupInterval time.Duration) {
    stopCleanup = make(chan struct{})
    cleanupTicker = time.NewTicker(cleanupInterval)

    go func() {
        for {
            select {
            case <-cleanupTicker.C:
                performCleanup(threshold)
            case <-stopCleanup:
                cleanupTicker.Stop()
                return
            }
        }
    }()
}

func StopExecutionStoreCleanup() {
    if stopCleanup != nil {
        close(stopCleanup)
    }
}

func performCleanup(threshold time.Duration) {
    now := timeProvider.Now()
    ExecutionStore.Lock()
    defer ExecutionStore.Unlock()

    for execID, execResult := range ExecutionStore.Executions {
        if execResult.CompletedAt != "" {
            completedAt, err := time.Parse(time.RFC3339, execResult.CompletedAt)
            if err == nil && now.Sub(completedAt) > threshold {
                delete(ExecutionStore.Executions, execID)
                log.Printf("Deleted execution result %s due to expiration", execID)
            }
        }
    }
}

func AddExecution(execID string, result *ExecutionResult) {
    ExecutionStore.Lock()
    defer ExecutionStore.Unlock()
    ExecutionStore.Executions[execID] = result
}

func GetExecution(execID string) (*ExecutionResult, bool) {
    ExecutionStore.RLock()
    defer ExecutionStore.RUnlock()
    result, exists := ExecutionStore.Executions[execID]
    return result, exists
}