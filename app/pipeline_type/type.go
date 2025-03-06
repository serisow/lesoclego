package pipeline_type

import "github.com/serisow/lesocle/services/llm_service"

// Used essentially to detect if pipeline might run, so we fetch minimal data
type ScheduledPipeline struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	ScheduledTime int64  `json:"scheduled_time"`
}

// The full pipeline data
type Pipeline struct {
	ID                string         `json:"id"`
	Label             string         `json:"label"`
	Steps             []PipelineStep `json:"steps"`
	ScheduledTime     int64          `json:"scheduled_time"`
	ExecutionFailures int            `json:"execution_failures"`
	LLMServices       map[string]llm_service.LLMService
	Context           *Context
}

type PipelineStep struct {
	ID                 string                 `json:"id"`
	Type               string                 `json:"type"`
	Weight             int                    `json:"weight"`
	StepDescription    string                 `json:"step_description"`
	StepOutputKey      string                 `json:"step_output_key"`
	OutputType         string                 `json:"output_type"`
	RequiredSteps      string                 `json:"required_steps"`
	LLMConfig          string                 `json:"llm_config,omitempty"`
	Prompt             string                 `json:"prompt,omitempty"`
	Response           string                 `json:"response,omitempty"`
	UUID               string                 `json:"uuid"`
	LLMServiceConfig   map[string]interface{} `json:"llm_service,omitempty"`
	ActionConfig       string                 `json:"action_config,omitempty"`
	ActionDetails      *ActionDetails         `json:"action_details,omitempty"`
	GoogleSearchConfig *GoogleSearchConfig    `json:"google_search_config,omitempty"`
	NewsAPIConfig      *NewsAPIConfig         `json:"news_api_config,omitempty"`
	SearchInput        string                 `json:"search_input,omitempty"`
	// Drupal node data for social media step
	ArticleData       map[string]interface{} `json:"article_data,omitempty"`
	UploadImageConfig *UploadImageConfig     `json:"upload_image_config,omitempty"`
	UploadAudioConfig *UploadAudioConfig     `json:"upload_audio_config,omitempty"`
}

type ActionDetails struct {
	ID                string                 `json:"id"`
	Label             string                 `json:"label"`
	ActionService     string                 `json:"action_service"`
	ExecutionLocation string                 `json:"execution_location"`
	Configuration     map[string]interface{} `json:"configuration"`
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

type ContentSearchSettings struct {
	IncludeMetadata    int    `json:"include_metadata"`
	MinWordCount       string `json:"min_word_count"`
	ExcludeAlreadyUsed int    `json:"exclude_already_used"`
}

type NewsAPIDateRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type NewsAPIAdvancedParams struct {
	Language  string           `json:"language"`
	SortBy    string           `json:"sort_by"`
	PageSize  string           `json:"page_size"`
	DateRange NewsAPIDateRange `json:"date_range"`
}

type NewsAPIConfig struct {
	Query          string                `json:"query"`
	AdvancedParams NewsAPIAdvancedParams `json:"advanced_params"`
}

// UploadImageConfig holds configuration for upload image steps
type UploadImageConfig struct {
	FileID   int64   `json:"image_file_id"`
	FileURL  string  `json:"image_file_url"`
	FileURI  string  `json:"image_file_uri"`
	FileMime string  `json:"image_file_mime"`
	FileName string  `json:"image_file_name"`
	FileSize int64   `json:"image_file_size"`
	Duration float64 `json:"duration"`
	TextOverlay  map[string]interface{} `json:"text_overlay,omitempty"`
}

// UploadAudioConfig holds configuration for upload audio steps
type UploadAudioConfig struct {
	FileID       int64   `json:"audio_file_id"`
	FileURL      string  `json:"audio_file_url"`
	FileURI      string  `json:"audio_file_uri"`
	FileMime     string  `json:"audio_file_mime"`
	FileName     string  `json:"audio_file_name"`
	FileDuration float64 `json:"audio_file_duration"`
	FileSize     int64   `json:"audio_file_size"`
}
