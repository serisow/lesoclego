package upload_step

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

type ImageEnrichmentStepImpl struct {
    PipelineStep pipeline_type.PipelineStep
    Logger       *slog.Logger
}

func (s *ImageEnrichmentStepImpl) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    s.Logger.Info("Executing image enrichment step", 
        slog.String("step_id", s.PipelineStep.ID),
        slog.String("uuid", s.PipelineStep.UUID))

    // Find structured news content in the context from previous steps
    newsItems, err := s.findNewsItemsWithImages(pipelineContext)
    if err != nil {
        return fmt.Errorf("failed to find news items with images: %w", err)
    }
    
    // Get target news item based on configuration
    targetIndex := s.getTargetIndex(newsItems)
    // Ensure index is valid
    if targetIndex >= len(newsItems) {
        targetIndex = len(newsItems) - 1
    }
    
    newsItem := newsItems[targetIndex]
    s.Logger.Debug("Selected news item", 
        slog.Int("index", targetIndex),
        slog.String("headline", newsItem.Headline))
        
    // Process the news item and download image
    result, err := s.processNewsItem(newsItem)
    if err != nil {
        return fmt.Errorf("failed to process news item: %w", err)
    }
    
    // Store the result in context
    resultJSON, err := json.Marshal(result)
    if err != nil {
        return fmt.Errorf("error marshaling result: %w", err)
    }
    
    pipelineContext.SetStepOutput(s.PipelineStep.StepOutputKey, string(resultJSON))
    
    return nil
}

// NewsItem represents a processed news article with image data
type NewsItem struct {
    ArticleID    string     `json:"article_id"`
    Headline     string     `json:"headline"`
    Summary      string     `json:"summary"`
    ImagePrompt  string     `json:"image_prompt"`
    Caption      string     `json:"caption"`
    ImageInfo    ImageInfo  `json:"image_info"`
}

// ImageInfo represents image file information
type ImageInfo struct {
    FileID      int64   `json:"file_id"` 
    URI         string  `json:"uri"`
    URL         string  `json:"url"`
    MimeType    string  `json:"mime_type"`
    Filename    string  `json:"filename"`
    Size        int64   `json:"size"`
    Timestamp   int64   `json:"timestamp"`
}

// Find news items with images in the pipeline context
func (s *ImageEnrichmentStepImpl) findNewsItemsWithImages(pipelineContext *pipeline_type.Context) ([]NewsItem, error) {
    // First look in explicitly required steps
    if s.PipelineStep.RequiredSteps != "" {
        for _, stepKey := range strings.Split(s.PipelineStep.RequiredSteps, "\r\n") {
            stepKey = strings.TrimSpace(stepKey)
            if stepKey == "" {
                continue
            }
            
            if output, exists := pipelineContext.GetStepOutput(stepKey); exists {
                if items := parseNewsItems(output); len(items) > 0 {
                    return items, nil
                }
            }
        }
    }
    
    // If not found in required steps, search all outputs
    for key, value := range pipelineContext.StepOutputs {
        if items := parseNewsItems(value); len(items) > 0 {
            s.Logger.Debug("Found news items in step output", 
                slog.String("key", key),
                slog.Int("count", len(items)))
            return items, nil
        }
    }
    
    return nil, fmt.Errorf("no news items with images found in context")
}

// Parse news items from various formats
func parseNewsItems(value interface{}) []NewsItem {
    // Handle string output (common from LLM steps)
    if strValue, ok := value.(string); ok {
        // Clean JSON string (remove code block markers if present)
        cleaned := strings.TrimSpace(strValue)
        cleaned = strings.TrimPrefix(cleaned, "```json")
        cleaned = strings.TrimSuffix(cleaned, "```")
        cleaned = strings.TrimSpace(cleaned)
        
        var items []NewsItem
        if err := json.Unmarshal([]byte(cleaned), &items); err == nil && len(items) > 0 {
            return items
        }
    }
    
    // Handle direct unmarshalled array
    if items, ok := value.([]NewsItem); ok {
        return items
    }
    
    // Try to convert from generic map
    if mapValue, ok := value.(map[string]interface{}); ok {
        if data, ok := mapValue["data"].([]interface{}); ok {
            var items []NewsItem
            dataJSON, _ := json.Marshal(data)
            if err := json.Unmarshal(dataJSON, &items); err == nil {
                return items
            }
        }
    }
    
    return nil
}

// Process text blocks from template and news content
func (s *ImageEnrichmentStepImpl) processTextBlocks(newsItem NewsItem) []map[string]interface{} {
    var results []map[string]interface{}
    
    if s.PipelineStep.ImageEnrichmentConfig != nil && 
       s.PipelineStep.ImageEnrichmentConfig.TextBlocks != nil {
        
        for _, templateBlock := range s.PipelineStep.ImageEnrichmentConfig.TextBlocks {
            // Skip disabled blocks
            enabled, _ := getBoolValue(templateBlock, "enabled", false)
            if !enabled {
                continue
            }
            
            // Create processed block
            block := map[string]interface{}{
                "id":               getStringValue(templateBlock, "id", ""),
                "enabled":          true,
                "position":         getStringValue(templateBlock, "position", "center"),
                "font_size":        getStringValue(templateBlock, "font_size", "24"),
                "font_color":       getStringValue(templateBlock, "font_color", "white"),
                "background_color": getStringValue(templateBlock, "background_color", ""),
            }
            
            // Fill content based on block ID and default value check
            text := getStringValue(templateBlock, "text", "")
            defaultTexts := []string{"default title", "default subtitle", "default body", "default caption"}
            
            if text == "" || contains(defaultTexts, strings.ToLower(text)) {
                blockID := getStringValue(templateBlock, "id", "")
                switch blockID {
                case "title_block":
                    text = newsItem.Headline
                case "subtitle_block":
                    text = newsItem.Caption
                case "body_block":
                    text = newsItem.Summary
                case "caption_block":
                    text = newsItem.Caption
                }
            }
            
            // Format the text (truncate, add line breaks for readability)
            block["text"] = formatTextForVideo(text, getStringValue(templateBlock, "id", ""))
            
            // Add animation settings if present
            if animation, ok := templateBlock["animation"].(map[string]interface{}); ok {
                block["animation"] = map[string]interface{}{
                    "type":     getStringValue(animation, "type", "none"),
                    "duration": getFloatValue(animation, "duration", 1.0),
                    "delay":    getFloatValue(animation, "delay", 0.0),
                    "easing":   getStringValue(animation, "easing", "linear"),
                }
            }
            
            // Handle custom position if needed
            position := getStringValue(templateBlock, "position", "")
            if position == "custom" {
                block["custom_x"] = getStringValue(templateBlock, "custom_x", "0")
                block["custom_y"] = getStringValue(templateBlock, "custom_y", "0")
            }
            
            results = append(results, block)
        }
    }
    
    return results
}

// Modified function without unecessary file ID conversion
func (s *ImageEnrichmentStepImpl) processNewsItem(newsItem NewsItem) (map[string]interface{}, error) {
    // First, download the image if needed
    localImagePath, err := s.downloadImage(newsItem.ImageInfo)
    if err != nil {
        return nil, fmt.Errorf("failed to download image: %w", err)
    }
    
    // Process the text blocks based on news item content
    textBlocks := s.processTextBlocks(newsItem)
    
    // Get file stats
    fileInfo, err := os.Stat(localImagePath)
    if err != nil {
        return nil, fmt.Errorf("failed to get file info: %w", err)
    }
    
    // Generate a new file ID using current timestamp
    fileID := time.Now().UnixNano()
    
    // Format the result for featured_image output type
    result := map[string]interface{}{
        "file_id": fileID,
        "uri": localImagePath,
        "url": newsItem.ImageInfo.URL,
        "mime_type": detectMimeType(localImagePath, "image/jpeg"),
        "filename": filepath.Base(localImagePath),
        "size": fileInfo.Size(),
        "timestamp": time.Now().Unix(),
        "duration": s.PipelineStep.ImageEnrichmentConfig.Duration,
        "text_blocks": textBlocks,
    }
    
    return result, nil
}

// Download image to local storage
func (s *ImageEnrichmentStepImpl) downloadImage(imageInfo interface{}) (string, error) {
    // Extract URL from different possible formats
    var imageURL string
    
    switch info := imageInfo.(type) {
    case map[string]interface{}:
        if url, ok := info["url"].(string); ok && url != "" {
            imageURL = url
        } else if url, ok := info["uri"].(string); ok && 
                 (strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
            imageURL = url
        }
    case ImageInfo:
        imageURL = info.URL
    case string:
        // Direct URL string
        if strings.HasPrefix(info, "http") {
            imageURL = info
        }
    }
    
    if imageURL == "" {
        return "", fmt.Errorf("no valid image URL found")
    }
    
    // Create directory for downloaded images
    dir := filepath.Join("storage", "pipeline", "images", time.Now().Format("2006-01"))
    if err := os.MkdirAll(dir, 0755); err != nil {
        return "", fmt.Errorf("failed to create directory: %w", err)
    }
    
    // Generate filename for the downloaded image
    filename := fmt.Sprintf("image_%d_%s", time.Now().UnixNano(), filepath.Base(imageURL))
    outputPath := filepath.Join(dir, filename)
    
    // Download the image
    s.Logger.Debug("Downloading image", 
        slog.String("url", imageURL), 
        slog.String("to", outputPath))
    
    client := &http.Client{
        Timeout: 30 * time.Second,
    }
    
    resp, err := client.Get(imageURL)
    if err != nil {
        return "", fmt.Errorf("failed to download image: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("failed to download image, status: %d", resp.StatusCode)
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
        return "", fmt.Errorf("failed to save image data: %w", err)
    }
    
    s.Logger.Info("Successfully downloaded image", slog.String("path", outputPath))
    return outputPath, nil
}

func (s *ImageEnrichmentStepImpl) getTargetIndex(newsItems []NewsItem) int {
    // Use the step's weight for ordering
    stepWeight := s.PipelineStep.Weight
    targetIndex := stepWeight % len(newsItems)
    
    s.Logger.Debug("Selected news item based on weight", 
        slog.Int("weight", stepWeight),
        slog.Int("index", targetIndex),
        slog.Int("total_items", len(newsItems)))
        
    return targetIndex
}

// Helper function to detect mime type from path
func detectMimeType(path string, defaultMime string) string {
    ext := strings.ToLower(filepath.Ext(path))
    switch ext {
    case ".jpg", ".jpeg":
        return "image/jpeg"
    case ".png":
        return "image/png"
    case ".gif":
        return "image/gif"
    case ".webp":
        return "image/webp"
    case ".svg":
        return "image/svg+xml"
    }
    return defaultMime
}

// Helper functions to safely get values from a map
func getStringValue(m map[string]interface{}, key, defaultValue string) string {
    if val, ok := m[key].(string); ok {
        return val
    }
    return defaultValue
}

func getFloatValue(m map[string]interface{}, key string, defaultValue float64) float64 {
    switch v := m[key].(type) {
    case float64:
        return v
    case float32:
        return float64(v)
    case int:
        return float64(v)
    case string:
        if f, err := strconv.ParseFloat(v, 64); err == nil {
            return f
        }
    }
    return defaultValue
}

func getBoolValue(m map[string]interface{}, key string, defaultValue bool) (bool, bool) {
    switch v := m[key].(type) {
    case bool:
        return v, true
    case string:
        if v == "1" || strings.ToLower(v) == "true" {
            return true, true
        } else if v == "0" || strings.ToLower(v) == "false" {
            return false, true
        }
    case float64:
        if v == 1.0 {
            return true, true
        } else if v == 0.0 {
            return false, true
        }
    case int:
        if v == 1 {
            return true, true
        } else if v == 0 {
            return false, true
        }
    }
    return defaultValue, false
}

// Format text for video display
func formatTextForVideo(text string, blockType string) string {
    // Define limits based on block type
    maxChars := 100
    lineLength := 40
    
    switch blockType {
    case "title_block":
        maxChars = 80
        lineLength = 30
    case "subtitle_block":
        maxChars = 100
        lineLength = 40
    case "body_block":
        maxChars = 450
        lineLength = 100
    case "caption_block":
        maxChars = 120
        lineLength = 40
    }
    
    // Truncate if too long
    if len(text) > maxChars {
        // Try to cut at sentence boundary
        if idx := strings.LastIndex(text[:maxChars], ". "); idx > maxChars/2 {
            text = text[:idx+1]
        } else if idx := strings.LastIndex(text[:maxChars], " "); idx > 0 {
            // Cut at word boundary
            text = text[:idx] + "..."
        } else {
            // Last resort
            text = text[:maxChars] + "..."
        }
    }
    
    // Add line breaks for readability
    words := strings.Fields(text)
    if len(words) > 0 {
        var lines []string
        currentLine := words[0]
        
        for i := 1; i < len(words); i++ {
            if len(currentLine)+1+len(words[i]) > lineLength {
                lines = append(lines, currentLine)
                currentLine = words[i]
            } else {
                currentLine += " " + words[i]
            }
        }
        
        lines = append(lines, currentLine)
        text = strings.Join(lines, "\n")
    }
    
    return text
}

// Helper function to check if a string is in a slice
func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}

func (s *ImageEnrichmentStepImpl) GetType() string {
    return "image_enrichment_step"
}

