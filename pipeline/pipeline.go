package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/serisow/lesocle/action_step"
	"github.com/serisow/lesocle/config"
	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/plugin_registry"
	"github.com/serisow/lesocle/step"
)



var sendExecutionResultsFunc = SendExecutionResults

func ExecutePipeline(p *pipeline_type.Pipeline, registry *plugin_registry.PluginRegistry) error {
	ctx := context.Background()
    if p.Context == nil {
        p.Context = pipeline_type.NewContext()
    }

	results := make(map[string]interface{})
	pipelineStartTime := time.Now().Unix()


	for _, pipelineStep := range p.Steps {
		stepStartTime := time.Now().Unix()

		var step step.Step
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
            actionStep := &action_step.ActionStepImpl{PipelineStep: pipelineStep}

            // Assume ActionConfig contains the action service name
            actionServiceName := pipelineStep.ActionConfig

            actionServiceInstance, ok := registry.GetActionService(actionServiceName)
            if !ok {
                return fmt.Errorf("unknown Action service: %s", actionServiceName)
            }

            actionStep.ActionServiceInstance = actionServiceInstance
            step = actionStep

		case "google_search":
            step = &GoogleSearchStepImpl{PipelineStep: pipelineStep}
		default:
			return fmt.Errorf("unknown step type: %s", pipelineStep.Type)
		}

		err = step.Execute(ctx, p.Context)
		stepEndTime := time.Now().Unix()

		output, _ := p.Context.GetStepOutput(pipelineStep.StepOutputKey)
		stepResult := map[string]interface{}{
			"step_uuid":        pipelineStep.UUID,
			"step_description": pipelineStep.StepDescription,
			"status":           "completed",
			"start_time":       stepStartTime,
			"end_time":         stepEndTime,
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
	err := sendExecutionResultsFunc(p.ID, results, pipelineStartTime, pipelineEndTime)
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