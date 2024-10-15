package scheduler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

)

/*
Scheduling System Limitations and User Responsibilities

This scheduling system does not automatically adjust for Daylight Saving Time (DST)
transitions or leap year peculiarities. Users are responsible for:

1. Adjusting pipeline schedules around DST changes in their respective time zones.
2. Handling February 29th schedules in leap years if needed.
3. Setting critical job schedules at hours not typically affected by DST (e.g., 11:00 AM).

The system operates on the time provided without applying time zone conversions or DST
adjustments. All times are treated as specified in the user's local time zone.

These tests ensure the system behaves correctly given these limitations and user
responsibilities. For detailed guidelines, refer to the user documentation.
*/

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

