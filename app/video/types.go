package video

import (
	"context"
	
	"github.com/serisow/lesocle/pipeline_type"
)

// FileInfo represents standardized file information
type FileInfo struct {
	FileID      int64                  `json:"file_id"`
	URI         string                 `json:"uri"`
	URL         string                 `json:"url"`
	MimeType    string                 `json:"mime_type"`
	Filename    string                 `json:"filename"`
	Size        int64                  `json:"size"`
	Timestamp   int64                  `json:"timestamp"`
	Duration    float64                `json:"duration,omitempty"`
	StepKey     string                 `json:"step_key,omitempty"`
}

// VideoParams contains all parameters needed for video generation
type VideoParams struct {
	ImageFiles         []*FileInfo
	AudioFile          *FileInfo
	OutputPath         string
	Resolution         string
	TransitionType     string
	TransitionDuration float64
	ImageDurations     []float64
	Config             map[string]interface{}
	PipelineContext    *pipeline_type.Context
}

// FileManager handles file operations
type FileManager interface {
	FindFilesByOutputType(ctx context.Context, pipelineContext *pipeline_type.Context, outputType string) ([]*FileInfo, error)
	FindFileByOutputType(ctx context.Context, pipelineContext *pipeline_type.Context, outputType string) (*FileInfo, error)
	DownloadFile(ctx context.Context, fileURL string, fileType string) (string, error)
	URIToFilePath(uri string) string
}

// FFmpegExecutor handles video generation commands
type FFmpegExecutor interface {
	GetAudioDuration(filePath string) (float64, error)
	CreateMultiImageVideo(params VideoParams) error
	GetResolution(quality string) string
	CalculateImageDurations(sourceData interface{}, audioDuration float64, transitionDuration float64) []float64
}

// TextProcessor handles text overlay processing
type TextProcessor interface {
	ProcessTextContent(text string, pipelineContext *pipeline_type.Context) string
	BuildTextOverlayFilter(config map[string]interface{}, text string, position string) string
	GetTextPosition(position string, resolution string, customCoords map[string]string) string
	ValidateTextOverlayConfig(config map[string]interface{}) bool
}

// VideoGenerationError represents errors in the video generation process
type VideoGenerationError struct {
	Stage string
	Err   error
}

func (e *VideoGenerationError) Error() string {
	return e.Stage + ": " + e.Err.Error()
}