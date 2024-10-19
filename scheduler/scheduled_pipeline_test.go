package scheduler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/plugin_registry"
)


func TestShouldRun(t *testing.T) {
	tests := []struct {
		name     string
		pipeline ScheduledPipeline
		now      time.Time
		want     bool
	}{
		// One-time schedule tests
		{
			name: "One-time schedule - Should run (never run before)",
			pipeline: ScheduledPipeline{
				ScheduleType:  "one_time",
				ScheduledTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC).Unix(),
				LastRunTime:   0,
			},
			now:  time.Date(2023, 1, 1, 12, 2, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "One-time schedule - Should not run (already run)",
			pipeline: ScheduledPipeline{
				ScheduleType:  "one_time",
				ScheduledTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC).Unix(),
				LastRunTime:   time.Date(2023, 1, 1, 12, 1, 0, 0, time.UTC).Unix(),
			},
			now:  time.Date(2023, 1, 1, 12, 2, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "One-time schedule - Should not run (before scheduled time)",
			pipeline: ScheduledPipeline{
				ScheduleType:  "one_time",
				ScheduledTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC).Unix(),
				LastRunTime:   0,
			},
			now:  time.Date(2023, 1, 1, 11, 59, 0, 0, time.UTC),
			want: false,
		},
		// Recurring daily schedule tests
		{
			name: "Daily recurring - Should run (never run before)",
			pipeline: ScheduledPipeline{
				ScheduleType:       "recurring",
				RecurringFrequency: "daily",
				RecurringTime:      "14:30",
				LastRunTime:        0,
			},
			now:  time.Date(2023, 1, 1, 14, 32, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "Daily recurring - Should run (run yesterday)",
			pipeline: ScheduledPipeline{
				ScheduleType:       "recurring",
				RecurringFrequency: "daily",
				RecurringTime:      "14:30",
				LastRunTime:        time.Date(2023, 1, 1, 14, 30, 0, 0, time.UTC).Unix(),
			},
			now:  time.Date(2023, 1, 2, 14, 32, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "Daily recurring - Should not run (already run today)",
			pipeline: ScheduledPipeline{
				ScheduleType:       "recurring",
				RecurringFrequency: "daily",
				RecurringTime:      "14:30",
				LastRunTime:        time.Date(2023, 1, 1, 14, 30, 0, 0, time.UTC).Unix(),
			},
			now:  time.Date(2023, 1, 1, 14, 35, 0, 0, time.UTC),
			want: false,
		},
		// Recurring weekly schedule tests
		{
			name: "Weekly recurring - Should run (Monday, never run before)",
			pipeline: ScheduledPipeline{
				ScheduleType:       "recurring",
				RecurringFrequency: "weekly",
				RecurringTime:      "10:00",
				LastRunTime:        0,
			},
			now:  time.Date(2023, 1, 2, 10, 2, 0, 0, time.UTC), // Monday
			want: true,
		},
		{
			name: "Weekly recurring - Should not run (Tuesday, run yesterday)",
			pipeline: ScheduledPipeline{
				ScheduleType:       "recurring",
				RecurringFrequency: "weekly",
				RecurringTime:      "10:00",
				LastRunTime:        time.Date(2023, 1, 2, 10, 0, 0, 0, time.UTC).Unix(), // Monday
			},
			now:  time.Date(2023, 1, 3, 10, 0, 0, 0, time.UTC), // Tuesday
			want: false,
		},
		// Recurring monthly schedule tests
		{
			name: "Monthly recurring - Should run (1st of month, never run before)",
			pipeline: ScheduledPipeline{
				ScheduleType:       "recurring",
				RecurringFrequency: "monthly",
				RecurringTime:      "00:01",
				LastRunTime:        0,
			},
			now:  time.Date(2023, 2, 1, 0, 2, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "Monthly recurring - Should not run (1st of month, run today)",
			pipeline: ScheduledPipeline{
				ScheduleType:       "recurring",
				RecurringFrequency: "monthly",
				RecurringTime:      "00:01",
				LastRunTime:        time.Date(2023, 2, 1, 0, 1, 0, 0, time.UTC).Unix(),
			},
			now:  time.Date(2023, 2, 1, 0, 5, 0, 0, time.UTC),
			want: false,
		},
		// Edge cases
		{
			name: "Invalid schedule type",
			pipeline: ScheduledPipeline{
				ScheduleType: "invalid",
				LastRunTime:  0,
			},
			now:  time.Now(),
			want: false,
		},
		{
			name: "Recurring with invalid time format",
			pipeline: ScheduledPipeline{
				ScheduleType:       "recurring",
				RecurringFrequency: "daily",
				RecurringTime:      "25:00", // Invalid time
				LastRunTime:        0,
			},
			now:  time.Now(),
			want: false,
		},

		{
			name: "One-time schedule - Should run (scheduled in the past, scheduler starts late)",
			pipeline: ScheduledPipeline{
				ScheduleType:  "one_time",
				ScheduledTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC).Unix(),
				LastRunTime:   0,
			},
			now:  time.Date(2023, 1, 1, 14, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "One-time schedule - Should not run (manually run after scheduled time)",
			pipeline: ScheduledPipeline{
				ScheduleType:  "one_time",
				ScheduledTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC).Unix(),
				LastRunTime:   time.Date(2023, 1, 1, 13, 0, 0, 0, time.UTC).Unix(),
			},
			now:  time.Date(2023, 1, 1, 14, 0, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "Daily recurring - Should run (missed yesterday's window)",
			pipeline: ScheduledPipeline{
				ScheduleType:       "recurring",
				RecurringFrequency: "daily",
				RecurringTime:      "14:30",
				LastRunTime:        time.Date(2023, 1, 1, 14, 30, 0, 0, time.UTC).Unix(),
			},
			now:  time.Date(2023, 1, 3, 14, 34, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "Daily recurring - Should not run (manually run today outside window)",
			pipeline: ScheduledPipeline{
				ScheduleType:       "recurring",
				RecurringFrequency: "daily",
				RecurringTime:      "14:30",
				LastRunTime:        time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC).Unix(),
			},
			now:  time.Date(2023, 1, 1, 14, 32, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "Edge case - Daylight Saving Time transition",
			pipeline: ScheduledPipeline{
				ScheduleType:       "recurring",
				RecurringFrequency: "daily",
				RecurringTime:      "02:30", // This time might be ambiguous during DST transition
				LastRunTime:        time.Date(2023, 3, 11, 2, 30, 0, 0, time.UTC).Unix(),
			},
			now:  time.Date(2023, 3, 12, 2, 31, 0, 0, time.UTC), // Day of DST transition in the US
			want: true,
		},

	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pipeline.ShouldRun(tt.now); got != tt.want {
				t.Errorf("ScheduledPipeline.ShouldRun() = %v, want %v", got, tt.want)
			}
		})
	}
}


func TestFetchScheduledPipelines(t *testing.T) {
	// Setup a mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the correct endpoint is being called
		if r.URL.Path != "/pipelines/scheduled" {
			t.Errorf("Expected to request '/pipelines/scheduled', got: %s", r.URL.Path)
		}
		// Respond with a mock JSON response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		mockResponse := `[
			{
				"id": "pipeline1",
				"label": "Daily Pipeline",
				"schedule_type": "recurring",
				"recurring_frequency": "daily",
				"recurring_time": "10:00",
				"last_run_time": 1635724800
			},
			{
				"id": "pipeline2",
				"label": "One-time Pipeline",
				"schedule_type": "one_time",
				"scheduled_time": 1635811200,
				"last_run_time": 0
			}
		]`
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer mockServer.Close()

	// Create a scheduler with the mock server URL
	s := &Scheduler{
		apiEndpoint: mockServer.URL,
	}

	// Call the function we're testing
	pipelines, err := s.fetchScheduledPipelines()

	// Check for errors
	if err != nil {
		t.Fatalf("fetchScheduledPipelines returned an error: %v", err)
	}

	// Check the number of pipelines returned
	if len(pipelines) != 2 {
		t.Errorf("Expected 2 pipelines, got %d", len(pipelines))
	}

	// Check the details of the first pipeline
	if pipelines[0].ID != "pipeline1" || pipelines[0].ScheduleType != "recurring" || pipelines[0].RecurringTime != "10:00" {
		t.Errorf("First pipeline details do not match expected values")
	}

	// Check the details of the second pipeline
	if pipelines[1].ID != "pipeline2" || pipelines[1].ScheduleType != "one_time" || pipelines[1].ScheduledTime != 1635811200 {
		t.Errorf("Second pipeline details do not match expected values")
	}
}


func TestExecutePipeline(t *testing.T) {
    var wg sync.WaitGroup
    wg.Add(1)

    // Mock fetchFullPipeline function
    mockFetchFullPipeline := func(id, apiEndpoint string) (pipeline_type.Pipeline, error) {
        if id != "test-pipeline" {
            t.Errorf("Expected pipeline ID 'test-pipeline', got '%s'", id)
        }
        return pipeline_type.Pipeline{
            ID:    "test-pipeline",
            Label: "Test Pipeline",
            Steps: []pipeline_type.PipelineStep{
                {
                    ID:   "step1",
                    Type: "mock_step",
                },
            },
            Context: pipeline_type.NewContext(),
        }, nil
    }

    // Mock executePipelineFunc function
    mockExecutePipelineFunc := func(executionID string, p *pipeline_type.Pipeline, registry *plugin_registry.PluginRegistry) error {
        if p.ID != "test-pipeline" {
            t.Errorf("Expected pipeline ID 'test-pipeline', got '%s'", p.ID)
        }
        return nil
    }

    // Create a scheduler with the mock functions
    s := &Scheduler{
        fetchPipelineFunc:    mockFetchFullPipeline,
        executePipelineFunc:  mockExecutePipelineFunc,
        runningPipelines:     make(map[string]struct{}),
        onPipelineComplete: func(pipelineID string) {
            wg.Done()
        },
    }

    // Ensure runningPipelines map is empty before the test
    s.runningPipelinesMutex.Lock()
    s.runningPipelines = make(map[string]struct{})
    s.runningPipelinesMutex.Unlock()

    // Execute the pipeline
    s.executePipeline("test-pipeline")

    // Wait for the pipeline execution and cleanup to finish
    done := make(chan struct{})
    go func() {
        wg.Wait()
        close(done)
    }()

    select {
    case <-done:
        // Execution and cleanup are complete
    case <-time.After(1 * time.Second):
        t.Fatal("Test timed out waiting for pipeline execution")
    }

    // Verify that the pipeline was removed from runningPipelines
    s.runningPipelinesMutex.Lock()
    _, exists := s.runningPipelines["test-pipeline"]
    s.runningPipelinesMutex.Unlock()
    if exists {
        t.Errorf("Pipeline 'test-pipeline' should have been removed from runningPipelines")
    }
}




func TestExecutePipelineConcurrency(t *testing.T) {
    var executionCount int32
    var wg sync.WaitGroup

    // Mock functions
    mockFetchFullPipeline := func(id, apiEndpoint string) (pipeline_type.Pipeline, error) {
        return pipeline_type.Pipeline{ID: id}, nil
    }

    mockExecutePipelineFunc := func(executionID string, p *pipeline_type.Pipeline, registry *plugin_registry.PluginRegistry) error {
        atomic.AddInt32(&executionCount, 1)
        time.Sleep(100 * time.Millisecond) // Simulate some work
        wg.Done()
        return nil
    }

    // Create scheduler with mock functions
    s := &Scheduler{
        fetchPipelineFunc:   mockFetchFullPipeline,
        executePipelineFunc: mockExecutePipelineFunc,
    }

    // Ensure runningPipelines map is empty before the test
    // Ensure runningPipelines map is empty before the test
    s.runningPipelinesMutex.Lock()
    s.runningPipelines = make(map[string]struct{})
    s.runningPipelinesMutex.Unlock()
    // We expect only one execution
    wg.Add(1)

    // Attempt to execute the same pipeline multiple times concurrently
    for i := 0; i < 5; i++ {
        s.executePipeline("test-pipeline")
    }

    // Wait for the execution to finish
    wg.Wait()

    // Check that executionCount is 1
    if executionCount != 1 {
        t.Errorf("Expected executionCount to be 1, got %d", executionCount)
    }
}


func TestExecutePipelineExecutionError(t *testing.T) {
    // Mock functions
    mockFetchFullPipeline := func(id, apiEndpoint string) (pipeline_type.Pipeline, error) {
        return pipeline_type.Pipeline{ID: id}, nil
    }

    // Mock executePipelineFunc function that returns an error
    mockExecutePipelineFunc := func(executionID string, p *pipeline_type.Pipeline, registry *plugin_registry.PluginRegistry) error {
        return fmt.Errorf("execution error")
    }

    // Create scheduler with mock functions
    s := &Scheduler{
        fetchPipelineFunc:    mockFetchFullPipeline,
        executePipelineFunc:  mockExecutePipelineFunc,
        runningPipelines:     make(map[string]struct{}), // Initialize the map
    }

    // Ensure runningPipelines map is empty before the test
    s.runningPipelinesMutex.Lock()
    s.runningPipelines = make(map[string]struct{})
    s.runningPipelinesMutex.Unlock()

    // Attempt to execute the pipeline
    s.executePipeline("test-pipeline")

    // Wait briefly to allow the goroutine to execute
    time.Sleep(100 * time.Millisecond)

    // Check that the pipeline was removed from runningPipelines even after error
    s.runningPipelinesMutex.Lock()
    _, exists := s.runningPipelines["test-pipeline"]
    s.runningPipelinesMutex.Unlock()
    if exists {
        t.Errorf("Pipeline 'test-pipeline' should have been removed from runningPipelines after execution error")
    }
}


func TestFetchFullPipeline(t *testing.T) {
    // Test cases
    tests := []struct {
        name           string
        pipelineID     string
        responseStatus int
        responseBody   string
        expectError    bool
        errorMsg       string
    }{
        {
            name:           "Successful Fetch",
            pipelineID:     "test-pipeline",
            responseStatus: http.StatusOK,
            responseBody: `{
                "id": "test-pipeline",
                "label": "Test Pipeline",
                "steps": [
                    {
                        "id": "step1",
                        "type": "mock_step"
                    }
                ]
            }`,
            expectError: false,
        },
        {
            name:           "HTTP 404 Not Found",
            pipelineID:     "nonexistent-pipeline",
            responseStatus: http.StatusNotFound,
            responseBody:   "Not Found",
            expectError:    true,
            errorMsg:       "HTTP request failed with status 404",
        },
        {
            name:           "Malformed JSON",
            pipelineID:     "malformed-pipeline",
            responseStatus: http.StatusOK,
            responseBody:   `{"id": "malformed-pipeline", "label": "Test Pipeline", "steps": [`,
            expectError:    true,
            errorMsg:       "failed to unmarshal JSON",
        },
        {
            name:           "Network Error",
            pipelineID:     "network-error-pipeline",
            responseStatus: http.StatusOK,
            responseBody:   "",
            expectError:    true,
            errorMsg:       "HTTP GET request failed",
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            // Setup mock server
            mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                expectedPath := fmt.Sprintf("/pipelines/%s", tc.pipelineID)
                if r.URL.Path != expectedPath {
                    t.Errorf("Expected request to %s, got %s", expectedPath, r.URL.Path)
                }

                // Simulate network error by closing the connection abruptly
                if tc.name == "Network Error" {
                    conn, _, err := w.(http.Hijacker).Hijack()
                    if err != nil {
                        t.Fatalf("Failed to hijack connection: %v", err)
                    }
                    conn.Close()
                    return
                }

                w.WriteHeader(tc.responseStatus)
                _, _ = w.Write([]byte(tc.responseBody))
            }))
            defer mockServer.Close()

            // Call the function under test
            p, err := fetchFullPipeline(tc.pipelineID, mockServer.URL)

            if tc.expectError {
                if err == nil {
                    t.Fatalf("Expected an error but got none")
                }
                if !contains(err.Error(), tc.errorMsg) {
                    t.Errorf("Expected error message to contain '%s', got '%s'", tc.errorMsg, err.Error())
                }
                return
            }

            if err != nil {
                t.Fatalf("Unexpected error: %v", err)
            }

            // Verify the Pipeline object
            if p.ID != tc.pipelineID {
                t.Errorf("Expected pipeline ID '%s', got '%s'", tc.pipelineID, p.ID)
            }
            if p.Context == nil {
                t.Errorf("Expected pipeline context to be initialized")
            }
            // Additional checks can be added here for other fields
        })
    }
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
    return strings.Contains(s, substr)
}
