package video

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/serisow/lesocle/pipeline_type"
)

// FileManagerImpl implements the FileManager interface
type FileManagerImpl struct {
	logger *slog.Logger
}

// NewFileManager creates a new file manager instance
func NewFileManager(logger *slog.Logger) FileManager {
	return &FileManagerImpl{
		logger: logger,
	}
}

// Updated FindFilesByOutputType method in FileManagerImpl struct
func (fm *FileManagerImpl) FindFilesByOutputType(ctx context.Context, pipelineContext *pipeline_type.Context, outputType string) ([]*FileInfo, error) {
	fm.logger.Info("Looking for files with output type", slog.String("output_type", outputType))

	var files []*FileInfo

	// Find all steps with matching output_type
	steps := pipelineContext.GetStepsByOutputType(outputType)
	fm.logger.Debug("Found steps with matching output type",
		slog.Int("count", len(steps)),
		slog.Any("step_ids", getStepIDs(steps)))

	// First, try to find by exact output_type match
	for _, step := range steps {
		if stepOutput, exists := pipelineContext.GetStepOutput(step.StepOutputKey); exists {
			fileInfo, err := fm.parseFileInfo(stepOutput, outputType)
			if err == nil {
				fileInfo.StepKey = step.StepOutputKey

				// Add duration
				if outputType == "featured_image" && step.UploadImageConfig != nil {
					fileInfo.Duration = step.UploadImageConfig.Duration
					// Extract text blocks if they exist
					if len(step.UploadImageConfig.TextBlocks) > 0 {
						fileInfo.TextBlocks = make([]TextBlock, 0, len(step.UploadImageConfig.TextBlocks))
						
						for _, blockData := range step.UploadImageConfig.TextBlocks {
							block := parseTextBlock(blockData)
							// Only add enabled blocks with non-empty text
							if block.Enabled && block.Text != "" {
								fileInfo.TextBlocks = append(fileInfo.TextBlocks, block)
							}
						}
						
						fm.logger.Debug("Extracted text blocks",
							slog.Int("count", len(fileInfo.TextBlocks)),
							slog.String("step_id", step.ID))
					}
				}

				files = append(files, fileInfo)
				fm.logger.Info("Found file info from step with matching output_type",
					slog.String("step_id", step.ID),
					slog.String("output_key", step.StepOutputKey))
			} else {
				fm.logger.Debug("Could not parse file info from step",
					slog.String("step_id", step.ID),
					slog.String("error", err.Error()))
			}
		}
	}

	// If no files found through direct output_type match, try known keys
	if len(files) == 0 && outputType == "featured_image" {
		// Look for keys containing "image_data"
		for key, value := range pipelineContext.StepOutputs {
			if strings.Contains(key, "image_data") {
				fileInfo, err := fm.parseFileInfo(value, outputType)
				if err == nil {
					fileInfo.StepKey = key
					
					// Try to find associated step to get text blocks and duration
					for _, step := range pipelineContext.Steps {
						if step.StepOutputKey == key && step.UploadImageConfig != nil {
							fileInfo.Duration = step.UploadImageConfig.Duration
							// Extract text blocks
							if len(step.UploadImageConfig.TextBlocks) > 0 {
								fileInfo.TextBlocks = make([]TextBlock, 0, len(step.UploadImageConfig.TextBlocks))
								
								for _, blockData := range step.UploadImageConfig.TextBlocks {
									block := parseTextBlock(blockData)
									// Only add enabled blocks with non-empty text
									if block.Enabled && block.Text != "" {
										fileInfo.TextBlocks = append(fileInfo.TextBlocks, block)
									}
								}
							}
							break
						}
					}

					files = append(files, fileInfo)
					fm.logger.Info("Found file info from key scan",
						slog.String("key", key),
						slog.String("uri", fileInfo.URI))
				}
			}
		}
	}

	// Sort files by their step weights if available
	if len(files) > 1 {
		fm.sortFilesByStepWeight(files, pipelineContext)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found with output type: %s", outputType)
	}

	return files, nil
}

// FindFileByOutputType looks for a single file matching a specific output type
func (fm *FileManagerImpl) FindFileByOutputType(ctx context.Context, pipelineContext *pipeline_type.Context, outputType string) (*FileInfo, error) {
	fm.logger.Info("Looking for file with output type", slog.String("output_type", outputType))

	// First approach: Look for steps that have the specific output_type
	steps := pipelineContext.GetStepsByOutputType(outputType)
	for _, step := range steps {
		if stepOutput, exists := pipelineContext.GetStepOutput(step.StepOutputKey); exists {
			fileInfo, err := fm.parseFileInfo(stepOutput, outputType)
			if err == nil {
				if outputType == "featured_image" && step.UploadImageConfig != nil && step.UploadImageConfig.Duration > 0 {
					fileInfo.Duration = step.UploadImageConfig.Duration
				}
				return fileInfo, nil
			}
		}
	}

	// Second approach: Try common step output keys
	if outputType == "audio_content" && pipelineContext.StepOutputs["audio_data"] != nil {
		fileInfo, err := fm.parseFileInfo(pipelineContext.StepOutputs["audio_data"], outputType)
		if err == nil {
			return fileInfo, nil
		}
	}

	// Final approach: Scan all outputs as a last resort
	for key, value := range pipelineContext.StepOutputs {
		if strings.Contains(key, "audio") {
			fileInfo, err := fm.parseFileInfo(value, outputType)
			if err == nil {
				fm.logger.Info("Found file info from step output scanning",
					slog.String("key", key),
					slog.String("uri", fileInfo.URI))
				return fileInfo, nil
			}
		}
	}

	return nil, fmt.Errorf("no file info found with output type: %s", outputType)
}

// URIToFilePath converts a URI to a file path
func (fm *FileManagerImpl) URIToFilePath(uri string) string {
	// Handle different URI formats
	if strings.HasPrefix(uri, "/") {
		// Already an absolute path
		return uri
	}

	// For file paths from upload steps, they should already be local paths
	if strings.HasPrefix(uri, "storage/") {
		// Return as is - it's a local path
		return uri
	}

	// Remove scheme if present (like file://)
	if strings.Contains(uri, "://") {
		uri = uri[strings.Index(uri, "://")+3:]
	}

	return uri
}

// DownloadFile downloads a file from a URL to a local directory
func (fm *FileManagerImpl) DownloadFile(ctx context.Context, fileURL string, fileType string) (string, error) {
	// Create a directory for downloaded files
	dir := filepath.Join("storage", "pipeline", fileType, time.Now().Format("2006-01"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate filename
	extension := "dat"
	if fileType == "images" {
		extension = "jpg"
	} else if fileType == "audio" {
		extension = "mp3"
	}

	filename := fmt.Sprintf("%s_%d.%s", fileType[:3], time.Now().UnixNano(), extension)
	outputPath := filepath.Join(dir, filename)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// Download the file
	fm.logger.Debug("Downloading file", slog.String("url", fileURL), slog.String("to", outputPath))

	resp, err := client.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download file, status: %d", resp.StatusCode)
	}

	// Create output file
	file, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Copy the content
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save file data: %w", err)
	}

	fm.logger.Info("Successfully downloaded file", slog.String("path", outputPath))
	return outputPath, nil
}

// parseFileInfo extracts file information from various formats
func (fm *FileManagerImpl) parseFileInfo(output interface{}, outputType string) (*FileInfo, error) {
	// Case 1: Direct URL (e.g., OpenAI image output)
	if url, ok := output.(string); ok {
		// Check if it's a URL that matches the expected type
		if outputType == "featured_image" && isImageURL(url) {
			fm.logger.Debug("Got direct image URL", slog.String("url", url))

			// Download the image to a local file
			localFilePath, err := fm.DownloadFile(context.Background(), url, "images")
			if err != nil {
				return nil, fmt.Errorf("failed to download image: %w", err)
			}

			// Create a file info structure for the downloaded image
			fileInfo := &FileInfo{
				FileID:      0,
				URI:         localFilePath,
				URL:         url,
				MimeType:    detectMimeType(url, "image/jpeg"),
				Filename:    filepath.Base(localFilePath),
				Size:        0,
				Timestamp:   time.Now().Unix(),
				TextBlocks:  []TextBlock{}, 
			}

			return fileInfo, nil
		}

		// Check if it's a JSON string
		if isJSON(url) {
			// Try to unmarshal with text blocks field
			var fileResponse struct {
				FileID      string      `json:"file_id"`
				URI         string      `json:"uri"`
				URL         string      `json:"url"`
				MimeType    string      `json:"mime_type"`
				Filename    string      `json:"filename"`
				Size        int64       `json:"size"`
				Duration    float64     `json:"duration,omitempty"`
				Timestamp   int64       `json:"timestamp"`
				TextBlocks  []TextBlock `json:"text_blocks,omitempty"`
			}

			if err := json.Unmarshal([]byte(url), &fileResponse); err == nil && fileResponse.URI != "" {
				// Convert to our standard FileInfo format
				var fileID int64 = 0
				if id, err := strconv.ParseInt(fileResponse.FileID, 10, 64); err == nil {
					fileID = id
				}

				fileInfo := &FileInfo{
					FileID:      fileID,
					URI:         fileResponse.URI,
					URL:         fileResponse.URL,
					MimeType:    fileResponse.MimeType,
					Filename:    fileResponse.Filename,
					Size:        fileResponse.Size,
					Duration:    fileResponse.Duration,
					Timestamp:   fileResponse.Timestamp,
					TextBlocks:  fileResponse.TextBlocks,
				}

				// Check if this matches the expected output type
				if outputType == "featured_image" && !isImageType(fileInfo.MimeType) && !strings.Contains(fileInfo.URI, "images") {
					return nil, fmt.Errorf("file info doesn't match expected image type")
				}

				if outputType == "audio_content" && !isAudioType(fileInfo.MimeType) && !strings.Contains(fileInfo.URI, "audio") {
					return nil, fmt.Errorf("file info doesn't match expected audio type")
				}

				return fileInfo, nil
			}


			// If that fails, try with the standard FileInfo struct as a fallback
			var fileInfo FileInfo
			if err := json.Unmarshal([]byte(url), &fileInfo); err != nil {
				return nil, fmt.Errorf("not valid file info JSON: %w", err)
			}

			// Validate the file info matches the expected output type
			if outputType == "featured_image" && !isImageType(fileInfo.MimeType) && !strings.Contains(fileInfo.URI, "images") {
				return nil, fmt.Errorf("file info doesn't match expected image type")
			}

			if outputType == "audio_content" && !isAudioType(fileInfo.MimeType) && !strings.Contains(fileInfo.URI, "audio") {
				return nil, fmt.Errorf("file info doesn't match expected audio type")
			}

			return &fileInfo, nil
		}

		return nil, fmt.Errorf("output is a string but not a valid file info format")
	}

	// Case 2: Map interface (could be a file info object)
	if mapData, ok := output.(map[string]interface{}); ok {
		// Check if it has the expected fields for a file info
		if uri, ok := mapData["uri"].(string); ok && uri != "" {
			fileInfo := FileInfo{
				URI: uri,
			}

			if url, ok := mapData["url"].(string); ok {
				fileInfo.URL = url
			}

			if mimeType, ok := mapData["mime_type"].(string); ok {
				fileInfo.MimeType = mimeType
			}

			if filename, ok := mapData["filename"].(string); ok {
				fileInfo.Filename = filename
			}

			// Try to extract FileID
			if fileID, ok := mapData["file_id"]; ok {
				switch v := fileID.(type) {
				case int:
					fileInfo.FileID = int64(v)
				case int64:
					fileInfo.FileID = v
				case float64:
					fileInfo.FileID = int64(v)
				case string:
					if id, err := strconv.ParseInt(v, 10, 64); err == nil {
						fileInfo.FileID = id
					}
				}
			}

			// Try to extract Size
			if size, ok := mapData["size"]; ok {
				switch v := size.(type) {
				case int:
					fileInfo.Size = int64(v)
				case int64:
					fileInfo.Size = v
				case float64:
					fileInfo.Size = int64(v)
				}
			}

			// Try to extract Duration if available
			if duration, ok := mapData["duration"].(float64); ok {
				fileInfo.Duration = duration
			}

			// Extract text blocks if available
			if textBlocksData, ok := mapData["text_blocks"].([]interface{}); ok {
				fileInfo.TextBlocks = make([]TextBlock, 0, len(textBlocksData))
				
				for _, blockInterface := range textBlocksData {
					if blockMap, ok := blockInterface.(map[string]interface{}); ok {
						block := parseTextBlock(blockMap)
						if block.Enabled && block.Text != "" {
							fileInfo.TextBlocks = append(fileInfo.TextBlocks, block)
						}
					}
				}
			}

			// Validate the file info matches the expected output type
			if outputType == "featured_image" && !isImageType(fileInfo.MimeType) && !strings.Contains(fileInfo.URI, "images") {
				return nil, fmt.Errorf("file info doesn't match expected image type")
			}

			if outputType == "audio_content" && !isAudioType(fileInfo.MimeType) && !strings.Contains(fileInfo.URI, "audio") {
				return nil, fmt.Errorf("file info doesn't match expected audio type")
			}

			return &fileInfo, nil
		}
	}

	return nil, fmt.Errorf("output is not in a recognized file info format")
}

// sortFilesByStepWeight sorts the files based on their step weights
func (fm *FileManagerImpl) sortFilesByStepWeight(files []*FileInfo, pipelineContext *pipeline_type.Context) {
	// Create a map of step output key to weight
	weightMap := make(map[string]int)
	for _, step := range pipelineContext.Steps {
		weightMap[step.StepOutputKey] = step.Weight
	}

	// Sort files based on their step weights
	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			iWeight := weightMap[files[i].StepKey]
			jWeight := weightMap[files[j].StepKey]

			if iWeight > jWeight {
				files[i], files[j] = files[j], files[i]
			}
		}
	}
}

// parseTextBlock converts a map to a TextBlock struct
func parseTextBlock(data map[string]interface{}) TextBlock {
    block := TextBlock{
        ID:              getStringValue(data, "id", ""),
        Text:            getStringValue(data, "text", ""),
        Position:        getStringValue(data, "position", "center"),
        FontSize:        getStringValue(data, "font_size", "10"),
        FontColor:       getStringValue(data, "font_color", "white"),
        FontFamily:      getStringValue(data, "font_family", "default"),  // New field
        FontStyle:       getStringValue(data, "font_style", "normal"),    // New field
        BackgroundColor: getStringValue(data, "background_color", ""),
    }
    
    // Parse enabled flag
    enabled := false
    if e, ok := data["enabled"].(bool); ok {
        enabled = e
    } else if e, ok := data["enabled"].(string); ok {
        enabled = e == "1" || strings.ToLower(e) == "true"
    } else if e, ok := data["enabled"].(float64); ok {
        enabled = e == 1
    } else if e, ok := data["enabled"].(int); ok {
        enabled = e == 1
    }
    block.Enabled = enabled
    
    // Extract custom position coordinates if present
    if data["custom_x"] != nil {
        block.CustomX = fmt.Sprintf("%v", data["custom_x"])
    }
    if data["custom_y"] != nil {
        block.CustomY = fmt.Sprintf("%v", data["custom_y"])
    }
    
    // Extract animation settings if present
    if animData, ok := data["animation"].(map[string]interface{}); ok {
        block.Animation = &TextAnimation{
            Type:     getStringValue(animData, "type", "none"),
            Easing:   getStringValue(animData, "easing", "linear"),
        }
        
        // Parse duration
        if duration, ok := animData["duration"].(float64); ok {
            block.Animation.Duration = duration
        } else if durationStr, ok := animData["duration"].(string); ok {
            if d, err := strconv.ParseFloat(durationStr, 64); err == nil {
                block.Animation.Duration = d
            } else {
                block.Animation.Duration = 1.0 // Default value
            }
        } else {
            block.Animation.Duration = 1.0 // Default value
        }
        
        // Parse delay
        if delay, ok := animData["delay"].(float64); ok {
            block.Animation.Delay = delay
        } else if delayStr, ok := animData["delay"].(string); ok {
            if d, err := strconv.ParseFloat(delayStr, 64); err == nil {
                block.Animation.Delay = d
            } else {
                block.Animation.Delay = 0.0 // Default value
            }
        } else {
            block.Animation.Delay = 0.0 // Default value
        }
    }
    
    return block
}