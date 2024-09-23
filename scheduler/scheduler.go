package scheduler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/serisow/lesocle/pipeline"
)

type Scheduler struct {
	apiEndpoint   string
	checkInterval time.Duration
	registry      *pipeline.PluginRegistry
}

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

		now := time.Now().Unix()
		for _, sp := range scheduledPipelines {
			if sp.ScheduledTime <= now+5 { // 5-second buffer
				go s.executePipeline(sp.ID)
			}
		}

		time.Sleep(s.checkInterval)
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

func (s *Scheduler) fetchScheduledPipelines() ([]pipeline.ScheduledPipeline, error) {
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

    var scheduledPipelines []pipeline.ScheduledPipeline
    err = json.Unmarshal(body, &scheduledPipelines)
    if err != nil {
        return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
    }

    return scheduledPipelines, nil
}

func (s *Scheduler) executePipeline(pipelineID string) {
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