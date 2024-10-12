package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/serisow/lesocle/config"
	"github.com/serisow/lesocle/pipeline/llm_service"
)

// Used essentially to detect if pipeline might run, so we fetch minimal data
type ScheduledPipeline struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	ScheduledTime int64  `json:"scheduled_time"`
}

// The full pipeline data
type Pipeline struct {
	ID            string         `json:"id"`
	Label         string         `json:"label"`
	Steps         []PipelineStep `json:"steps"`
	ScheduledTime int64          `json:"scheduled_time"`
	LLMServices   map[string]llm_service.LLMService
	Context       *Context
}

type PipelineStep struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	Weight           int                    `json:"weight"`
	StepDescription  string                 `json:"step_description"`
	StepOutputKey    string                 `json:"step_output_key"`
	OutputType       string                 `json:"output_type"`
    RequiredSteps    string                 `json:"required_steps"`
	LLMConfig        string                 `json:"llm_config,omitempty"`
	Prompt           string                 `json:"prompt,omitempty"`
	Response         string                 `json:"response,omitempty"`
	UUID             string                 `json:"uuid"`
	LLMServiceConfig map[string]interface{} `json:"llm_service,omitempty"`
	ActionConfig     string                 `json:"action_config,omitempty"`
	GoogleSearchConfig *GoogleSearchConfig   `json:"google_search_config,omitempty"`
}

type GoogleSearchConfig struct {
    Query          string             `json:"query"`
    Category       string             `json:"category"`
    AdvancedParams GoogleSearchParams `json:"advanced_params"`
}

type GoogleSearchParams struct {
    NumResults   string `json:"num_results"`
    DateRestrict string `json:"date_restrict"`
    Sort         string `json:"sort"`
    Language     string `json:"language"`
    Country      string `json:"country"`
    SiteSearch   string `json:"site_search"`
    FileType     string `json:"file_type"`
    SafeSearch   string `json:"safe_search"`
}

func ExecutePipeline(p *Pipeline, registry *PluginRegistry) error {
	ctx := context.Background()
    if p.Context == nil {
        p.Context = NewContext()
    }

	results := make(map[string]interface{})
	pipelineStartTime := time.Now().Unix()


	for _, pipelineStep := range p.Steps {
		stepStartTime := time.Now().Unix()

		var step Step
		var err error

		switch pipelineStep.Type {
		case "llm_step":
			llmStep := &LLMStepImpl{PipelineStep: pipelineStep}

			serviceName, ok := pipelineStep.LLMServiceConfig["service_name"].(string)
			if !ok {
				return fmt.Errorf("service_name not found in llm_service configuration for step %s", pipelineStep.ID)
			}

			llmServiceInstance, ok := registry.GetLLMService(serviceName)
			if !ok {
				return fmt.Errorf("unknown LLM service: %s", serviceName)
			}

			llmStep.LLMServiceInstance = llmServiceInstance
			step = llmStep

		case "action_step":
			step = &ActionStepImpl{PipelineStep: pipelineStep}
		case "google_search":
            step = &GoogleSearchStepImpl{PipelineStep: pipelineStep}
		default:
			return fmt.Errorf("unknown step type: %s", pipelineStep.Type)
		}

		err = step.Execute(ctx, p.Context)
		stepEndTime := time.Now().Unix()
		stepDuration := stepEndTime - stepStartTime

		output, _ := p.Context.GetStepOutput(pipelineStep.StepOutputKey)
		stepResult := map[string]interface{}{
			"step_uuid":        pipelineStep.UUID,
			"step_description": pipelineStep.StepDescription,
			"status":           "completed",
			"start_time":       stepStartTime,
			"end_time":         stepEndTime,
			"duration":         stepDuration,
			"step_type":        pipelineStep.Type,
			"sequence":         pipelineStep.Weight,
			"data":             output,
			"output_type":      pipelineStep.OutputType,
			"error_message":    "",
		}

		if err != nil {
			stepResult["status"] = "failed"
			stepResult["error_message"] = err.Error()
			stepResult["data"] = fmt.Sprintf("Error: %v", err)
		}

		results[pipelineStep.UUID] = stepResult
	}

	pipelineEndTime := time.Now().Unix()

	// Send execution results to Drupal
	err := SendExecutionResults(p.ID, results, pipelineStartTime, pipelineEndTime)
	if err != nil {
		return fmt.Errorf("error sending execution results: %w", err)
	}
	
	return nil
}

func SendExecutionResults(pipelineID string, results map[string]interface{}, startTime, endTime int64) error {
	cfg := config.Load()

    apiEndpoint := fmt.Sprintf("%s/pipeline/%s/execution-result", cfg.APIEndpoint, pipelineID)

	executionData := map[string]interface{}{
        "pipeline_id": pipelineID,
        "start_time": startTime,
        "end_time": endTime,
        "step_results": results,
    }

    jsonData, err := json.Marshal(executionData)

    if err != nil {
        return fmt.Errorf("error marshaling results: %w", err)
    }

    req, err := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(jsonData))
    if err != nil {
        return fmt.Errorf("error creating request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")
    //req.SetBasicAuth(config.DrupalUsername, config.DrupalPassword)

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("error sending results: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
    }

    return nil
}