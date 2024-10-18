package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/serisow/lesocle/pipeline"
	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/plugin_registry"
	"github.com/serisow/lesocle/scheduler"
)

type PipelineHandler struct {
	APIEndpoint string
	Registry    *plugin_registry.PluginRegistry
}

func NewPipelineHandler(apiEndpoint string, registry *plugin_registry.PluginRegistry) *PipelineHandler {
	return &PipelineHandler{
		APIEndpoint: apiEndpoint,
		Registry:    registry,
	}
}

func (h *PipelineHandler) ExecutePipeline(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    pipelineID := vars["id"]

    // Parse user input from request body
    var requestBody struct {
        UserInput string `json:"user_input"`
    }
    if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    // Fetch the full pipeline
    fullPipeline, err := scheduler.FetchFullPipeline(pipelineID, h.APIEndpoint)
    if err != nil {
        http.Error(w, fmt.Sprintf("Failed to fetch pipeline: %v", err), http.StatusInternalServerError)
        return
    }

    // Check if the pipeline is allowed to be executed on demand
    if !isPipelineExecutableOnDemand(fullPipeline) {
        http.Error(w, "This pipeline is not configured for on-demand execution", http.StatusForbidden)
        return
    }

    // Execute the pipeline with user input
    go func() {
        // Set the user input in the pipeline's context
        if fullPipeline.Context == nil {
            fullPipeline.Context = pipeline_type.NewContext()
        }
        fullPipeline.Context.SetStepOutput("user_input", requestBody.UserInput)

        err := pipeline.ExecutePipeline(&fullPipeline, h.Registry)
        if err != nil {
            fmt.Printf("Error executing pipeline %s: %v\n", pipelineID, err)
        }
    }()

    // Respond to the client
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(map[string]string{"message": "Pipeline execution started"})
}

// isPipelineExecutableOnDemand is a placeholder function
// In the future, this will check if the pipeline is flagged for on-demand execution
func isPipelineExecutableOnDemand(p pipeline_type.Pipeline) bool {
	// For now, we'll assume all pipelines are executable on-demand
	// This should be replaced with actual logic when the Drupal side is updated
	return true
}