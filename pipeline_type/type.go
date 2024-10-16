package pipeline_type

import "github.com/serisow/lesocle/llm_service"

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