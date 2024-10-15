package scheduler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/serisow/lesocle/pipeline"
)

type Scheduler struct {
	apiEndpoint   string
	checkInterval time.Duration
	registry      *pipeline.PluginRegistry
}

type ScheduledPipeline struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	ScheduleType     string `json:"schedule_type"`
	ScheduledTime    int64  `json:"scheduled_time"`
	RecurringFrequency string `json:"recurring_frequency"`
	RecurringTime    string `json:"recurring_time"`
    LastRunTime        int64  `json:"last_run_time"`

}

// Prevent multiple instance of the same pipeline running at the same time.
// Solve potential data race.
var runningPipelines sync.Map

func New(apiEndpoint string, checkInterval time.Duration, registry *pipeline.PluginRegistry) *Scheduler {
	return &Scheduler{
		apiEndpoint:   apiEndpoint,
		checkInterval: checkInterval,
		registry:      registry,
	}
}

func (s *Scheduler) Start() {
	log.Println("Starting pipeline scheduler...")
	for {
		scheduledPipelines, err := s.fetchScheduledPipelines()
		if err != nil {
			log.Printf("Error fetching scheduled pipelines: %v", err)
			time.Sleep(s.checkInterval)
			continue
		}

		now := time.Now()
		for _, sp := range scheduledPipelines {
			if sp.ShouldRun(now) {
				go s.executePipeline(sp.ID)
			}
		}

		time.Sleep(s.checkInterval)
	}
}

func (s *Scheduler) fetchScheduledPipelines() ([]*ScheduledPipeline, error) {
	url := fmt.Sprintf("%s/%s", s.apiEndpoint, "pipelines/scheduled")
    resp, err := http.Get(url)
    if err != nil {
        return nil, fmt.Errorf("HTTP GET request failed: %v", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read response body: %v", err)
    }

    var scheduledPipelines []*ScheduledPipeline
    err = json.Unmarshal(body, &scheduledPipelines)
    if err != nil {
        return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
    }

    return scheduledPipelines, nil
}

func (s *Scheduler) executePipeline(pipelineID string) {
    if _, loaded := runningPipelines.LoadOrStore(pipelineID, struct{}{}); loaded {
        // Pipeline is already running
        return
    }
    defer runningPipelines.Delete(pipelineID)

    fullPipeline, err := s.fetchFullPipeline(pipelineID)
    if err != nil {
        log.Printf("Error fetching full pipeline %s: %v", pipelineID, err)
        return
    }

    err = pipeline.ExecutePipeline(&fullPipeline, s.registry)
    if err != nil {
        log.Printf("Error executing pipeline %s: %v", pipelineID, err)
    } else {
        log.Printf("Successfully executed pipeline %s", pipelineID)
    }
}

func (s *Scheduler) fetchFullPipeline(id string) (pipeline.Pipeline, error) {
    url := fmt.Sprintf("%s/%s/%s", s.apiEndpoint, "pipelines", id)
    resp, err := http.Get(url)
    if err != nil {
        return pipeline.Pipeline{}, fmt.Errorf("HTTP GET request failed: %v", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return pipeline.Pipeline{}, fmt.Errorf("failed to read response body: %v", err)
    }

    var p pipeline.Pipeline
    err = json.Unmarshal(body, &p)
    if err != nil {
        return p, fmt.Errorf("failed to unmarshal JSON: %v", err)
    }
    p.Context = pipeline.NewContext()
    return p, nil
}


func (sp *ScheduledPipeline) ShouldRun(now time.Time) bool {
	switch sp.ScheduleType {
	case "one_time":
		scheduledTime := time.Unix(sp.ScheduledTime, 0)
		if sp.LastRunTime == 0 {
			// If never run, check if it's time to run
			return now.After(scheduledTime) || now.Equal(scheduledTime)
		}
		lastRunTime := time.Unix(sp.LastRunTime, 0)
		return now.After(scheduledTime) && lastRunTime.Before(scheduledTime)
	case "recurring":
		scheduleTime, err := time.Parse("15:04", sp.RecurringTime)
		if err != nil {
			return false
		}
		
		// Create a time window: 5 minutes before and 5 minutes after the scheduled time
		scheduledDateTime := time.Date(now.Year(), now.Month(), now.Day(), scheduleTime.Hour(), scheduleTime.Minute(), 0, 0, now.Location())
		windowStart := scheduledDateTime.Add(-5 * time.Minute)
		windowEnd := scheduledDateTime.Add(5 * time.Minute)
		
		isWithinWindow := now.After(windowStart) && now.Before(windowEnd)
		
		if sp.LastRunTime == 0 {
			// If never run, it should run if within the time window
			return isWithinWindow
		}
		
		lastRunTime := time.Unix(sp.LastRunTime, 0)
		hasNotRunToday := lastRunTime.Before(now.Truncate(24 * time.Hour))
		
		switch sp.RecurringFrequency {
		case "daily":
			return isWithinWindow && hasNotRunToday
		case "weekly":
			return now.Weekday() == time.Monday && isWithinWindow && hasNotRunToday
		case "monthly":
			return now.Day() == 1 && isWithinWindow && hasNotRunToday
		}
	}
	return false
}