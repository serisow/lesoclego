package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/serisow/lesocle/action_step"
	"github.com/serisow/lesocle/config"
	"github.com/serisow/lesocle/llm_step"
	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/plugin_registry"
)



var SendExecutionResultsFunc = SendExecutionResults

func ExecutePipeline(p *pipeline_type.Pipeline, registry *plugin_registry.PluginRegistry) error {
    ctx := context.Background()
    if p.Context == nil {
        p.Context = pipeline_type.NewContext()
    }

    results := make(map[string]interface{})
    pipelineStartTime := time.Now().Unix()

    for _, pipelineStep := range p.Steps {
        stepStartTime := time.Now().Unix()

        // Get the step instance from the registry
        step, err := registry.GetStepInstance(pipelineStep.Type)
        if err != nil {
            return fmt.Errorf("unknown step type: %s", pipelineStep.Type)
        }

        // Set the PipelineStep field directly
        switch s := step.(type) {
        case *llm_step.LLMStepImpl:
            s.PipelineStep = pipelineStep
            // Additional setup for LLM service
            serviceName, ok := pipelineStep.LLMServiceConfig["service_name"].(string)
            if !ok {
                return fmt.Errorf("service_name not found in llm_service configuration for step %s", pipelineStep.ID)
            }
            llmServiceInstance, ok := registry.GetLLMService(serviceName)
            if !ok {
                return fmt.Errorf("unknown LLM service: %s", serviceName)
            }
            s.LLMServiceInstance = llmServiceInstance
        case *action_step.ActionStepImpl:
            s.PipelineStep = pipelineStep
            // Additional setup for action service
            actionServiceName := pipelineStep.ActionConfig
            actionServiceInstance, ok := registry.GetActionService(actionServiceName)
            if !ok {
                return fmt.Errorf("unknown Action service: %s", actionServiceName)
            }
            s.ActionServiceInstance = actionServiceInstance
        default:
            // Attempt to set the PipelineStep field directly
            if err := setPipelineStepField(step, pipelineStep); err != nil {
                return fmt.Errorf("cannot set PipelineStep for step type %s: %v", pipelineStep.Type, err)
            }
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
			// Return the error to stop pipeline execution
			return err
		}

		results[pipelineStep.UUID] = stepResult
	}

	pipelineEndTime := time.Now().Unix()

	// Send execution results to Drupal
	err := SendExecutionResultsFunc(p.ID, results, pipelineStartTime, pipelineEndTime)
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

// Helper function to set the PipelineStep field via reflection
func setPipelineStepField(step interface{}, pipelineStep pipeline_type.PipelineStep) error {
    v := reflect.ValueOf(step)
    if v.Kind() == reflect.Ptr {
        v = v.Elem()
    }
    field := v.FieldByName("PipelineStep")
    if !field.IsValid() {
        return fmt.Errorf("field PipelineStep not found")
    }
    if !field.CanSet() {
        return fmt.Errorf("field PipelineStep cannot be set")
    }
    field.Set(reflect.ValueOf(pipelineStep))
    return nil
}