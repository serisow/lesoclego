package action_service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/services/llm_service"
)

const (
	NewsItemImageGeneratorServiceName = "news_item_image_generator"
)

type NewsItemImageGeneratorActionService struct {
	logger     *slog.Logger
	llmServiceManager map[string]llm_service.LLMService
}

// NewsItemWithImage represents a news item with image generation info
type NewsItemWithImage struct {
	ArticleID    string      `json:"article_id"`
	Headline     string      `json:"headline"`
	Summary      string      `json:"summary"`
	Content      string      `json:"content,omitempty"`
	ImagePrompt  string      `json:"image_prompt"`
	Caption      string      `json:"caption,omitempty"`
	ImageInfo    interface{} `json:"image_info"`
	ImageError   string      `json:"image_error,omitempty"`
}

func NewNewsItemImageGeneratorActionService(logger *slog.Logger) *NewsItemImageGeneratorActionService {
	service := &NewsItemImageGeneratorActionService{
		logger: logger,
		llmServiceManager: make(map[string]llm_service.LLMService),
	}

	// Pre-register the OpenAI image service
	service.RegisterLLMService("openai_image", llm_service.NewOpenAIImageService(logger))
	
	return service
}

func (s *NewsItemImageGeneratorActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
	if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
		return "", fmt.Errorf("missing action configuration for NewsItemImageGeneratorAction")
	}

	// Extract configuration parameters
	config := step.ActionDetails.Configuration
	imageGenerator := getStringValue(config, "image_generator", "openai_image")
	imageConfigID := getStringValue(config, "image_config", "")
	imageSize := getStringValue(config, "image_size", "1024x1024")
	concurrentLimit := getIntValue(config, "concurrent_limit", 3)
	retryCount := getIntValue(config, "retry_count", 2)

	s.logger.Info("Starting news item image generation",
		slog.String("step_id", step.ID),
		slog.String("image_generator", imageGenerator),
		slog.String("image_size", imageSize),
		slog.Int("concurrent_limit", concurrentLimit),
		slog.Int("retry_count", retryCount))

	// Find the LLM service instance
	llmServiceInstance, ok := s.llmServiceManager[imageGenerator]
	if !ok {
		return "", fmt.Errorf("image generation service not found: %s", imageGenerator)
	}

	// Find structured news content data in the context
	newsItems, err := s.findNewsContentData(pipelineContext)
	if err != nil {
		return "", err
	}

	if len(newsItems) == 0 {
		return "", fmt.Errorf("no news items found in context")
	}
	
	// Process news items in batches based on concurrent limit
	var processedItems []NewsItemWithImage
	batches := chunkSlice(newsItems, concurrentLimit)

	for batchIdx, batch := range batches {
		s.logger.Debug("Processing batch",
			slog.Int("batch", batchIdx+1),
			slog.Int("total_batches", len(batches)),
			slog.Int("batch_size", len(batch)))

		var wg sync.WaitGroup
		var mu sync.Mutex
		batchResults := make([]NewsItemWithImage, len(batch))

		for i, item := range batch {
			wg.Add(1)
			go func(idx int, newsItem NewsItemWithImage) {
				defer wg.Done()

				// Skip if no image prompt
				if newsItem.ImagePrompt == "" {
					s.logger.Warn("Missing image prompt for article",
						slog.String("article_id", newsItem.ArticleID))
					
					// Store the article without image info
					mu.Lock()
					newsItem.ImageInfo = nil
					batchResults[idx] = newsItem
					mu.Unlock()
					return
				}

				// Extract LLM service configuration
				configParams := make(map[string]interface{})
				
				// Add required parameters for OpenAI image service
				configParams["service_name"] = imageGenerator
				configParams["image_size"] = imageSize
				
				// Find the correct LLM configuration
				for _, step := range pipelineContext.Steps {
					if step.LLMServiceConfig != nil && step.StepOutputKey == imageConfigID {
						// Use this step's LLM service config
						configParams = step.LLMServiceConfig
						// Make sure to set the image size
						configParams["image_size"] = imageSize
						break
					}
				}
				
				// If we don't have required fields, try to use default configuration
				if _, ok := configParams["api_url"]; !ok {
					// Check for "openai_image" service's standard config
					// Default OpenAI DALL-E API URL
					configParams["api_url"] = "https://api.openai.com/v1/images/generations"
				}
				
				if _, ok := configParams["api_key"]; !ok {
					// Try to get API key from environment
					apiKey := os.Getenv("OPENAI_API_KEY")
					if apiKey != "" {
						configParams["api_key"] = apiKey
					} else {
						s.logger.Error("OpenAI API key not found in config or environment",
							slog.String("article_id", newsItem.ArticleID))
						
						// Store error in result
						mu.Lock()
						newsItem.ImageInfo = nil
						newsItem.ImageError = "API key not found in config or environment"
						batchResults[idx] = newsItem
						mu.Unlock()
						return
					}
				}
				
				// Ensure model name is set for DALL-E
				if _, ok := configParams["model_name"]; !ok {
					configParams["model_name"] = "dall-e-3"
				}
				
				// Attempt to generate image with retries
				var success bool
				var errorMsg string
				var imageResult string

				for attempt := 0; attempt <= retryCount && !success; attempt++ {
					if attempt > 0 {
						s.logger.Warn("Retrying image generation",
							slog.String("article_id", newsItem.ArticleID),
							slog.Int("attempt", attempt),
							slog.Int("max_attempts", retryCount),
							slog.String("error", errorMsg))
						time.Sleep(2 * time.Second) // Wait before retry
					}

					// Call the LLM service with proper error handling
					func() {
						// Use recover to catch any panics
						defer func() {
							if r := recover(); r != nil {
								errorMsg = fmt.Sprintf("Panic in LLM service: %v", r)
								s.logger.Error("Panic while calling LLM service",
									slog.String("article_id", newsItem.ArticleID),
									slog.Any("panic", r))
							}
						}()
						
						// Make the actual call
						result, err := llmServiceInstance.CallLLM(ctx, configParams, newsItem.ImagePrompt)
						if err == nil {
							imageResult = result
							success = true
						} else {
							errorMsg = err.Error()
						}
					}()
				}

				mu.Lock()
				if success {
					// Parse the image result
					imageInfo, err := parseImageResult(imageResult)
					if err != nil {
						newsItem.ImageInfo = nil
						newsItem.ImageError = fmt.Sprintf("Failed to parse image result: %s", err.Error())
					} else {
						newsItem.ImageInfo = imageInfo
					}
				} else {
					newsItem.ImageInfo = nil
					newsItem.ImageError = errorMsg
					s.logger.Error("Image generation failed after retries",
						slog.String("article_id", newsItem.ArticleID),
						slog.Int("retries", retryCount),
						slog.String("error", errorMsg))
				}
				batchResults[idx] = newsItem
				mu.Unlock()
			}(i, item)
		}

		wg.Wait()
		processedItems = append(processedItems, batchResults...)

		// Add a small delay between batches to avoid rate limiting
		if len(batches) > 1 && batchIdx < len(batches)-1 {
			time.Sleep(1 * time.Second)
		}
	}
	// Return the results as JSON
	result, err := json.Marshal(processedItems)
	if err != nil {
		return "", fmt.Errorf("error marshaling results: %w", err)
	}

	s.logger.Info("News item image generation completed",
		slog.Int("total_processed", len(processedItems)))

	return string(result), nil
}

// parseImageResult attempts to parse the image generation result
func parseImageResult(imageResult string) (interface{}, error) {
	// Handle empty results
	if imageResult == "" {
		return nil, fmt.Errorf("empty image result")
	}
	
	// Try to parse as JSON
	var imageInfo interface{}
	err := json.Unmarshal([]byte(imageResult), &imageInfo)
	if err != nil {
		// If it's not JSON, it might be a direct URL
		if strings.HasPrefix(imageResult, "http") && 
		   (strings.Contains(imageResult, ".jpg") || 
		    strings.Contains(imageResult, ".png") || 
		    strings.Contains(imageResult, ".webp")) {
			// Return as a simple URL object
			return map[string]string{
				"url": imageResult,
			}, nil
		}
		return nil, fmt.Errorf("invalid JSON in image result: %w", err)
	}
	
	// If we have a map with a URL field, that's what we want
	if infoMap, ok := imageInfo.(map[string]interface{}); ok {
		// Check if we have a data array (OpenAI format)
		if dataArray, ok := infoMap["data"].([]interface{}); ok && len(dataArray) > 0 {
			if firstItem, ok := dataArray[0].(map[string]interface{}); ok {
				if url, ok := firstItem["url"].(string); ok && url != "" {
					// Return just the important part
					return map[string]string{
						"url": url,
					}, nil
				}
			}
		}
		
		// Check for direct URL field
		if url, ok := infoMap["url"].(string); ok && url != "" {
			return infoMap, nil
		}
	}
	
	return imageInfo, nil
}

// findNewsContentData looks for structured news content in the pipeline context
func (s *NewsItemImageGeneratorActionService) findNewsContentData(pipelineContext *pipeline_type.Context) ([]NewsItemWithImage, error) {
	// Log all step outputs for debugging
	s.logger.Debug("Searching for structured news content in pipeline context",
		slog.Any("available_step_keys", getMapKeys(pipelineContext.StepOutputs)))

	// First, look for steps with output_type="structured_news"
	steps := pipelineContext.GetStepsByOutputType("structured_news")
	if len(steps) > 0 {
		s.logger.Debug("Found steps with structured_news output_type",
			slog.Int("count", len(steps)))
			
		// Check each step's output
		for _, step := range steps {
			if output, exists := pipelineContext.GetStepOutput(step.StepOutputKey); exists {
				s.logger.Debug("Checking output from step with structured_news output_type",
					slog.String("step_id", step.ID),
					slog.String("step_output_key", step.StepOutputKey))
				
				if newsItems := tryParseNewsItems(output); newsItems != nil {
					s.logger.Info("Found structured news content in step with structured_news output_type",
						slog.String("step_id", step.ID),
						slog.Int("items_count", len(newsItems)))
					return newsItems, nil
				}
			}
		}
	}

	// If not found via output_type, try all step outputs
	for key, value := range pipelineContext.StepOutputs {
		s.logger.Debug("Checking step output for structured news content",
			slog.String("step_key", key))
			
		if newsItems := tryParseNewsItems(value); newsItems != nil {
			s.logger.Info("Found structured news content in step output",
				slog.String("step_key", key),
				slog.Int("items_count", len(newsItems)))
			return newsItems, nil
		}
	}

	// If we have steps with output_type="structured_news" but couldn't parse their output,
	// log their actual output for debugging
	if len(steps) > 0 {
		for _, step := range steps {
			if output, exists := pipelineContext.GetStepOutput(step.StepOutputKey); exists {
				outputStr := fmt.Sprintf("%v", output)
				if len(outputStr) > 500 {
					outputStr = outputStr[:500] + "..." // Truncate long outputs
				}
				s.logger.Error("Failed to parse output from step with structured_news output_type",
					slog.String("step_id", step.ID),
					slog.String("step_output_key", step.StepOutputKey),
					slog.String("output_preview", outputStr))
			}
		}
	}

	return nil, fmt.Errorf("no structured news content found in context. Make sure a previous step has generated structured news content")
}

// tryParseNewsItems attempts to parse news items from various formats
func tryParseNewsItems(value interface{}) []NewsItemWithImage {
	// Convert the output to string if it's not already
	outputStr, ok := value.(string)
	if !ok {
		// If not a string, try to marshal it
		if valueBytes, err := json.Marshal(value); err == nil {
			outputStr = string(valueBytes)
		} else {
			return nil
		}
	}

	// Clean the string by removing markdown code block delimiters
	cleaned := strings.TrimSpace(outputStr)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// Try to parse it as JSON array of news items
	var newsItems []NewsItemWithImage
	if err := json.Unmarshal([]byte(cleaned), &newsItems); err == nil && len(newsItems) > 0 {
		return newsItems
	}

	// Try to parse it as a step result with output_type field
	var stepResult struct {
		OutputType string          `json:"output_type"`
		Data       json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(cleaned), &stepResult); err == nil {
		if stepResult.OutputType == "structured_news" && len(stepResult.Data) > 0 {
			var newsItems []NewsItemWithImage
			if err := json.Unmarshal(stepResult.Data, &newsItems); err == nil && len(newsItems) > 0 {
				return newsItems
			}
		}
	}
	
	// Try to parse it as a generic JSON array and map to our structure
	var genericArray []map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &genericArray); err == nil && len(genericArray) > 0 {
		// Check if this looks like news items by checking for key fields
		hasImagePrompt := false
		for _, item := range genericArray {
			if _, ok := item["image_prompt"]; ok {
				hasImagePrompt = true
				break
			}
		}
		
		if hasImagePrompt {
			// Convert to our struct
			newsItems := make([]NewsItemWithImage, 0, len(genericArray))
			for _, item := range genericArray {
				newsItem := NewsItemWithImage{}
				
				// Map fields with type conversion
				if id, ok := item["article_id"]; ok {
					switch v := id.(type) {
					case string:
						newsItem.ArticleID = v
					case float64:
						newsItem.ArticleID = fmt.Sprintf("%.0f", v)
					case int:
						newsItem.ArticleID = fmt.Sprintf("%d", v)
					}
				}
				
				if headline, ok := item["headline"].(string); ok {
					newsItem.Headline = headline
				}
				
				if summary, ok := item["summary"].(string); ok {
					newsItem.Summary = summary
				}
				
				if content, ok := item["content"].(string); ok {
					newsItem.Content = content
				}
				
				if imagePrompt, ok := item["image_prompt"].(string); ok {
					newsItem.ImagePrompt = imagePrompt
				}
				
				if caption, ok := item["caption"].(string); ok {
					newsItem.Caption = caption
				}
				
				newsItems = append(newsItems, newsItem)
			}
			
			if len(newsItems) > 0 {
				return newsItems
			}
		}
	}

	return nil
}

// Helper function to get map keys for logging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// RegisterLLMService allows registering an LLM service with this action service
func (s *NewsItemImageGeneratorActionService) RegisterLLMService(name string, service llm_service.LLMService) {
	s.llmServiceManager[name] = service
}

func (s *NewsItemImageGeneratorActionService) CanHandle(actionService string) bool {
	return actionService == NewsItemImageGeneratorServiceName
}

// chunkSlice divides a slice into chunks of a specified size
func chunkSlice(slice []NewsItemWithImage, chunkSize int) [][]NewsItemWithImage {
	if chunkSize <= 0 {
		return [][]NewsItemWithImage{slice}
	}
	
	chunks := make([][]NewsItemWithImage, 0, (len(slice)+chunkSize-1)/chunkSize)
	
	for i := 0; i < len(slice); i += chunkSize {
		end := i + chunkSize
		if end > len(slice) {
			end = len(slice)
		}
		chunks = append(chunks, slice[i:end])
	}
	
	return chunks
}