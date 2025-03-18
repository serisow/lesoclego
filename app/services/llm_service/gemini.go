package llm_service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
    envConfig "github.com/serisow/lesocle/config"
)

type GeminiService struct {
    httpClient *http.Client
    logger     *slog.Logger
}

func NewGeminiService(logger *slog.Logger) *GeminiService {
    return &GeminiService{
        httpClient: &http.Client{Timeout: 120 * time.Second},
        logger:     logger,
    }
}

func (s *GeminiService) CallLLM(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
    maxRetries := 3
    retryDelay := 5 * time.Second

    // Check if this is an image generation request based on model name
    modelName, ok := config["model_name"].(string)
    if !ok {
        return "", fmt.Errorf("model_name not found in config")
    }
    
    // Look for any indication this is an image generation request
    isImageRequest := strings.Contains(strings.ToLower(modelName), "image") || 
                      modelName == "gemini-2.0-flash-exp-image-generation"

    for attempt := 1; attempt <= maxRetries; attempt++ {
        var response string
        var err error

        if isImageRequest {
            response, err = s.callGeminiImageGeneration(ctx, config, prompt)
        } else {
            response, err = s.callGemini(ctx, config, prompt)
        }

        if err == nil {
            return response, nil
        }

        if attempt == maxRetries {
            s.logger.Error("Error calling Gemini API after multiple attempts",
                slog.Int("attempts", maxRetries),
                slog.String("error",err.Error()))
            return "", fmt.Errorf("failed to call Gemini API after %d attempts: %w", maxRetries, err)
        }

        s.logger.Warn("Attempt failed, retrying",
            slog.Int("attempt", attempt),
            slog.Duration("retryDelay", retryDelay),
            slog.String("error", err.Error()))
        time.Sleep(retryDelay)
    }

    return "", fmt.Errorf("failed to call Gemini API after exhausting all retry attempts")
}

func (s *GeminiService) callGemini(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
    apiURL, ok := config["api_url"].(string)
    if !ok {
        return "", fmt.Errorf("api_url not found in config")
    }

    apiKey, ok := config["api_key"].(string)
    if !ok {
        return "", fmt.Errorf("api_key not found in config")
    }

    url := fmt.Sprintf("%s?key=%s", apiURL, apiKey)

    params, ok := config["parameters"].(map[string]interface{})
    if !ok {
        params = make(map[string]interface{})
    }

    payload := map[string]interface{}{
        "contents": []map[string]interface{}{
            {
                "role": "user",
                "parts": []map[string]string{
                    {"text": prompt},
                },
            },
        },
        "generationConfig": map[string]interface{}{
            "temperature":      safeParseFloat(params["temperature"], 1.0),
            "topK":             safeParseFloat(params["top_k"], 40),
            "topP":             safeParseFloat(params["top_p"], 0.95),
            "maxOutputTokens":  safeParseFloat(params["max_tokens"], 8192.0),
            "responseMimeType": "text/plain",
        },
    }

    requestBody, err := json.Marshal(payload)
    if err != nil {
        return "", fmt.Errorf("error marshaling request body: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
    if err != nil {
        return "", fmt.Errorf("error creating request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")

    resp, err := s.httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("error making request: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("error reading response body: %w", err)
    }

    var result map[string]interface{}
    if err := json.Unmarshal(body, &result); err != nil {
        return "", fmt.Errorf("error unmarshaling response: %w", err)
    }

    candidates, ok := result["candidates"].([]interface{})
    if !ok || len(candidates) == 0 {
        return "", fmt.Errorf("unexpected response format from Gemini API")
    }

    content, ok := candidates[0].(map[string]interface{})["content"].(map[string]interface{})
    if !ok {
        return "", fmt.Errorf("content not found in Gemini API response")
    }

    parts, ok := content["parts"].([]interface{})
    if !ok || len(parts) == 0 {
        return "", fmt.Errorf("parts not found in Gemini API response")
    }

    text, ok := parts[0].(map[string]interface{})["text"].(string)
    if !ok {
        return "", fmt.Errorf("text not found in Gemini API response")
    }

    return text, nil
}

func (s *GeminiService) callGeminiImageGeneration(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
    // Define the correct URL and model name for image generation
    correctModelName := "gemini-2.0-flash-exp-image-generation"
    correctAPIURL := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash-exp-image-generation:generateContent"

    // Override the API URL if it's not correct
    apiURL, ok := config["api_url"].(string)
    if !ok || !strings.Contains(apiURL, correctModelName) {
        apiURL = correctAPIURL
        s.logger.Info("Using correct Gemini image API URL", 
            slog.String("api_url", apiURL))
    }

    // Override the model name to ensure it's correct
    modelName := correctModelName
    config["model_name"] = modelName

    apiKey, ok := config["api_key"].(string)
    if !ok {
        return "", fmt.Errorf("api_key not found in config")
    }

    url := fmt.Sprintf("%s?key=%s", apiURL, apiKey)

    params, ok := config["parameters"].(map[string]interface{})
    if !ok {
        params = make(map[string]interface{})
    }

    // Construct proper payload for image generation
    payload := map[string]interface{}{
        "contents": []map[string]interface{}{
            {
                "role": "user",
                "parts": []map[string]string{
                    {"text": prompt},
                },
            },
        },
        "safetySettings": []map[string]string{
            {
                "category": "HARM_CATEGORY_HATE_SPEECH",
                "threshold": "BLOCK_NONE",
            },
            {
                "category": "HARM_CATEGORY_SEXUALLY_EXPLICIT",
                "threshold": "BLOCK_NONE",
            },
            {
                "category": "HARM_CATEGORY_DANGEROUS_CONTENT",
                "threshold": "BLOCK_NONE",
            },
            {
                "category": "HARM_CATEGORY_HARASSMENT",
                "threshold": "BLOCK_NONE",
            },
            {
                "category": "HARM_CATEGORY_CIVIC_INTEGRITY",
                "threshold": "BLOCK_NONE",
            },
        },
        "generationConfig": map[string]interface{}{
            "temperature":      safeParseFloat(params["temperature"], 1.0),
            "topK":             safeParseFloat(params["top_k"], 40),
            "topP":             safeParseFloat(params["top_p"], 0.95),
            "maxOutputTokens":  safeParseFloat(params["max_tokens"], 8192.0),
            "responseMimeType": "text/plain",
            "responseModalities": []string{"image", "text"},
        },
    }

    requestBody, err := json.Marshal(payload)
    if err != nil {
        return "", fmt.Errorf("error marshaling request body: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
    if err != nil {
        return "", fmt.Errorf("error creating request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")

    resp, err := s.httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("error making request: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("error reading response body: %w", err)
    }

    var result map[string]interface{}
    if err := json.Unmarshal(body, &result); err != nil {
        return "", fmt.Errorf("error unmarshaling response: %w", err)
    }

    // Extract image data from the response
    imageURL, err := s.extractImageFromResponse(result)
    if err != nil {
        // Check if we got a text response instead
        textResponse, textErr := s.extractTextFromResponse(result)
        if textErr == nil {
            s.logger.Warn("Gemini returned text instead of an image",
                slog.String("text", textResponse[:min(200, len(textResponse))]))
            
            // Return error information in the same format as we'd return an image
            errorResponse := map[string]interface{}{
                "error": true,
                "message": "Received text response instead of image",
                "text_response": textResponse,
                "timestamp": time.Now().Unix(),
            }
            
            jsonResponse, _ := json.Marshal(errorResponse)
            return string(jsonResponse), nil
        }
        
        return "", fmt.Errorf("failed to extract image from response: %w", err)
    }

    // Download and save the image
    return s.downloadAndSaveImage(ctx, imageURL, config)
}

func (s *GeminiService) extractImageFromResponse(response map[string]interface{}) (string, error) {
    // First check for error responses that might not have candidates field
    if errorInfo, ok := response["error"].(map[string]interface{}); ok {
        errorMessage := "Unknown error"
        if message, ok := errorInfo["message"].(string); ok {
            errorMessage = message
        }
        return "", fmt.Errorf("gemini API error: %s", errorMessage)
    }

    candidates, ok := response["candidates"].([]interface{})
    if !ok || len(candidates) == 0 {
        // If we have a promptFeedback section, check if it contains error info
        if promptFeedback, ok := response["promptFeedback"].(map[string]interface{}); ok {
            if blockReason, ok := promptFeedback["blockReason"].(string); ok {
                return "", fmt.Errorf("prompt blocked: %s", blockReason)
            }
            
            if safetyRatings, ok := promptFeedback["safetyRatings"].([]interface{}); ok && len(safetyRatings) > 0 {
                // Return a structured error with safety info
                errorResponse := map[string]interface{}{
                    "error": true,
                    "message": "Content blocked by safety settings",
                    "safety_feedback": promptFeedback,
                    "timestamp": time.Now().Unix(),
                }
                
                jsonResponse, _ := json.Marshal(errorResponse)
                return string(jsonResponse), nil
            }
        }
        
        // If there's no specific error info, return a general error
        return "", fmt.Errorf("no candidates found in response")
    }

    content, ok := candidates[0].(map[string]interface{})["content"].(map[string]interface{})
    if !ok {
        return "", fmt.Errorf("content not found in response")
    }

    parts, ok := content["parts"].([]interface{})
    if !ok || len(parts) == 0 {
        return "", fmt.Errorf("parts not found in response")
    }

    // Check each part for image data
    for _, part := range parts {
        partMap, ok := part.(map[string]interface{})
        if !ok {
            continue
        }

        // Check for inline data (base64 image)
        if inlineData, ok := partMap["inlineData"].(map[string]interface{}); ok {
            if data, ok := inlineData["data"].(string); ok && data != "" {
                return "data:image/png;base64," + data, nil
            }
        }

        // Check for file data (URL to image)
        if fileData, ok := partMap["fileData"].(map[string]interface{}); ok {
            if fileURI, ok := fileData["fileUri"].(string); ok && fileURI != "" {
                return fileURI, nil
            }
        }
    }

    return "", fmt.Errorf("no image data found in response")
}

func (s *GeminiService) extractTextFromResponse(response map[string]interface{}) (string, error) {
    candidates, ok := response["candidates"].([]interface{})
    if !ok || len(candidates) == 0 {
        return "", fmt.Errorf("no candidates found in response")
    }

    content, ok := candidates[0].(map[string]interface{})["content"].(map[string]interface{})
    if !ok {
        return "", fmt.Errorf("content not found in response")
    }

    parts, ok := content["parts"].([]interface{})
    if !ok || len(parts) == 0 {
        return "", fmt.Errorf("parts not found in response")
    }

    // Look for text part
    for _, part := range parts {
        partMap, ok := part.(map[string]interface{})
        if !ok {
            continue
        }

        if text, ok := partMap["text"].(string); ok && text != "" {
            return text, nil
        }
    }

    return "", fmt.Errorf("no text found in response")
}

func (s *GeminiService) downloadAndSaveImage(ctx context.Context, imageData string, config map[string]interface{}) (string, error) {
    var imageBytes []byte
    var err error

    // Check if it's a base64 data URI
    if strings.HasPrefix(imageData, "data:image/") {
        // Extract the base64 data
        base64Data := strings.Split(imageData, ",")[1]
        imageBytes, err = base64.StdEncoding.DecodeString(base64Data)
        if err != nil {
            return "", fmt.Errorf("error decoding base64 image: %w", err)
        }
    } else if strings.HasPrefix(imageData, "https://") {
        // It's a URL, download it
        req, err := http.NewRequestWithContext(ctx, "GET", imageData, nil)
        if err != nil {
            return "", fmt.Errorf("error creating download request: %w", err)
        }

        resp, err := s.httpClient.Do(req)
        if err != nil {
            return "", fmt.Errorf("error downloading image: %w", err)
        }
        defer resp.Body.Close()

        if resp.StatusCode != http.StatusOK {
            return "", fmt.Errorf("error downloading image, status: %d", resp.StatusCode)
        }

        imageBytes, err = io.ReadAll(resp.Body)
        if err != nil {
            return "", fmt.Errorf("error reading image data: %w", err)
        }
    } else {
        return "", fmt.Errorf("unsupported image data format")
    }

    // Create directory for storing images
    directory := filepath.Join("storage", "pipeline", "images", time.Now().Format("2006-01"))
    if err := os.MkdirAll(directory, 0755); err != nil {
        return "", fmt.Errorf("failed to create directory: %w", err)
    }
    // Generate file ID once and reuse it
    fileID := time.Now().UnixNano()
    // Generate unique filename
    filename := fmt.Sprintf("gemini_img_%d.png", fileID)
    outputPath := filepath.Join(directory, filename)

    // Save the image
    file, err := os.Create(outputPath)
    if err != nil {
        return "", fmt.Errorf("failed to create image file: %w", err)
    }
    defer file.Close()

    _, err = file.Write(imageBytes)
    if err != nil {
        return "", fmt.Errorf("failed to write image data: %w", err)
    }

    // Get file size
    fileInfo, err := file.Stat()
    if err != nil {
        return "", fmt.Errorf("failed to get file info: %w", err)
    }

    // Load config to get base URL
    cfg := envConfig.Load()
    
    // Create absolute download URL using the same fileID
    absoluteDownloadURL := fmt.Sprintf("%s/api/images/%d", cfg.ServiceBaseURL, fileID)
    
    // Get model name from config
    modelName, _ := config["model_name"].(string)
    
    // Use the same fileID in the result and add model/service info
    result := map[string]interface{}{
        "file_id": fileID,
        "uri": outputPath,
        "url": absoluteDownloadURL,
        "mime_type": "image/png",
        "filename": filename,
        "size": fileInfo.Size(),
        "timestamp": time.Now().Unix(),
        "model_name": modelName,
        "service": "gemini",
    }

    resultJSON, err := json.Marshal(result)
    if err != nil {
        return "", fmt.Errorf("error marshaling result: %w", err)
    }

    return string(resultJSON), nil
}