package scheduler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/serisow/lesocle/pipeline"
	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/plugin_registry"
)

const (
    MaxExecutionFailures = 3
)


type Scheduler struct {
	apiHost       string
	apiEndpoint   string
	checkInterval time.Duration
	registry      *plugin_registry.PluginRegistry
	fetchPipelineFunc  func(id,apiHost, apiEndpoint string) (pipeline_type.Pipeline, error)
    executePipelineFunc func(executionID string, p *pipeline_type.Pipeline, registry *plugin_registry.PluginRegistry) error
	onPipelineComplete func(pipelineID string)

	cronURL        string
    cronInterval   time.Duration

	runningPipelinesMutex sync.Mutex
    runningPipelines      map[string]struct{}

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


func New(apiHost, apiEndpoint string, checkInterval time.Duration, registry *plugin_registry.PluginRegistry, cronURL string, cronInterval time.Duration) *Scheduler {
	return &Scheduler{
		apiHost: apiHost,
		apiEndpoint:   apiEndpoint,
		checkInterval: checkInterval,
		registry:      registry,
		fetchPipelineFunc:  fetchFullPipeline,
        executePipelineFunc: pipeline.ExecutePipeline,
		runningPipelines:     make(map[string]struct{}),
		cronURL:        cronURL,
        cronInterval:   cronInterval,
	}
}

// Pull the one-time execution pipeline ever x minutes, x is set via .env file.
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

// Query the Drupal cron url, which trigger the Drupal cron every x minutes, set via the .env file.

func (s *Scheduler) StartCronTrigger() {
    if s.cronURL == "" {
        log.Println("Cron trigger disabled - no URL configured")
        return
    }
    
    log.Printf("Starting cron trigger for URL: %s with interval: %v", s.cronURL, s.cronInterval)
    ticker := time.NewTicker(s.cronInterval)
    
    go func() {
        for range ticker.C {
            if err := s.triggerCron(); err != nil {
                log.Printf("Error triggering Drupal cron: %v", err)
            }
        }
    }()
}


func (s *Scheduler) fetchScheduledPipelines() ([]*ScheduledPipeline, error) {
	url := fmt.Sprintf("%s/%s", s.apiEndpoint, "pipelines/scheduled")

    // Create a new request instead of using http.Get
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("HTTP request creation failed: %v", err)
    }
    
    // Add the Host header
    req.Host = s.apiHost
    
    // Use http.DefaultClient to make the request
    resp, err := http.DefaultClient.Do(req)
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
    s.runningPipelinesMutex.Lock()
    if _, exists := s.runningPipelines[pipelineID]; exists {
        s.runningPipelinesMutex.Unlock()
        return
    }
    s.runningPipelines[pipelineID] = struct{}{}
    s.runningPipelinesMutex.Unlock()

    fullPipeline, err := s.fetchPipelineFunc(pipelineID, s.apiHost, s.apiEndpoint)
    if err != nil {
        log.Printf("Error fetching full pipeline %s: %v", pipelineID, err)
        // Remove from runningPipelines since execution won't proceed
        s.runningPipelinesMutex.Lock()
        delete(s.runningPipelines, pipelineID)
        s.runningPipelinesMutex.Unlock()
        return
    }

	// Check failure count before executing
	if fullPipeline.ExecutionFailures >= MaxExecutionFailures {
		log.Printf("Pipeline %s has failed %d times consecutively. Skipping execution.", 
			pipelineID, fullPipeline.ExecutionFailures)
		s.runningPipelinesMutex.Lock()
		delete(s.runningPipelines, pipelineID)
		s.runningPipelinesMutex.Unlock()
		return
	}

    executionID := uuid.New().String()



    go func() {
        defer func() {
            s.runningPipelinesMutex.Lock()
            delete(s.runningPipelines, pipelineID)
            s.runningPipelinesMutex.Unlock()
			// Call the completion callback if it's set
			if s.onPipelineComplete != nil {
				s.onPipelineComplete(pipelineID)
			}
        }()

        err = s.executePipelineFunc(executionID, &fullPipeline, s.registry)
        if err != nil {
            log.Printf("Error executing pipeline %s: %v", pipelineID, err)
        } else {
            log.Printf("Successfully executed pipeline %s", pipelineID)
        }
    }()
}

func fetchFullPipeline(id, apiHost, apiEndpoint string) (pipeline_type.Pipeline, error) {
    url := fmt.Sprintf("%s/%s/%s", apiEndpoint, "pipelines", id)
    // Create a new request instead of using http.Get
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return pipeline_type.Pipeline{}, fmt.Errorf("HTTP request creation failed: %v", err)
    }
    
    // Add the Host header
    req.Host = apiHost
    
    // Use http.DefaultClient to make the request
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return pipeline_type.Pipeline{}, fmt.Errorf("HTTP GET request failed: %v", err)
    }
	
    defer resp.Body.Close()

    // Check for non-200 status codes
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body) // Read body to include in error message
        return pipeline_type.Pipeline{}, fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(body))
    }

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return pipeline_type.Pipeline{}, fmt.Errorf("failed to read response body: %v", err)
    }

    var p pipeline_type.Pipeline
    err = json.Unmarshal(body, &p)
    if err != nil {
        return p, fmt.Errorf("failed to unmarshal JSON: %v", err)
    }
    p.Context = pipeline_type.NewContext()
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

// FetchFullPipeline fetches a full pipeline by ID
func FetchFullPipeline(id, apiHost, apiEndpoint string) (pipeline_type.Pipeline, error) {
	return fetchFullPipeline(id, apiHost, apiEndpoint)
}

func (s *Scheduler) triggerCron() error {
    req, err := http.NewRequest("GET", s.cronURL, nil)
    if err != nil {
        return fmt.Errorf("failed to create cron request: %w", err)
    }
    
    // Add the Host header if needed
    if s.apiHost != "" {
        req.Host = s.apiHost
    }
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("failed to trigger cron: %w", err)
    }
    defer resp.Body.Close()
    
    // Consider both 200 OK and 204 No Content as success
    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("cron request failed with status %d: %s", resp.StatusCode, string(body))
    }
    
    log.Printf("Successfully triggered Drupal cron at %s", time.Now().Format(time.RFC3339))
    return nil
}