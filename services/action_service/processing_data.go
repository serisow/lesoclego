// This is an example to demonstrate that we can perfom computationnal heavy task in the go side
// and only delagating the Drupal side, Drupal related actions like creating or updating article etc.
// This open the door to an apt plateform to do very ambitious computation and data processing
// In main.go we would add this line: 
// registry.RegisterActionService("process_data_action", &action_service.ProcessDataActionService{})
// We can create as many as we want in this folder.
// Making sure to put the correct key match the Drupal side configuration of the pipeline

package action_service

import (
    "context"
    "github.com/serisow/lesocle/pipeline_type"
)

type ProcessDataActionService struct{}

func (a *ProcessDataActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    // Example of a local computation task
    processedData := performComplexTask()
    return processedData, nil
}

func performComplexTask() string {
    // Simulate a heavy computation task
    return "Processed data from Go"
}
