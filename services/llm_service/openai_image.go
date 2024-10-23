package llm_service

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
    "log/slog"
)

type OpenAIImageService struct {
    httpClient *http.Client
    logger     *slog.Logger
}

func NewOpenAIImageService(logger *slog.Logger) *OpenAIImageService {
    return &OpenAIImageService{
        httpClient: &http.Client{Timeout: 4800 * time.Second}, // 80 minutes timeout as per PHP version
        logger:     logger,
    }
}

func (s *OpenAIImageService) CallLLM(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
    maxRetries := 3
    retryDelay := 5 * time.Second

    for attempt := 1; attempt <= maxRetries; attempt++ {
        response, err := s.callOpenAIImage(ctx, config, prompt)
        if err == nil {
            return response, nil
        }

        // Check if error contains OpenAI error details
        if httpErr, ok := err.(*OpenAIHttpError); ok {
            if httpErr.StatusCode == 429 {
                s.logger.Error("OpenAI Image API quota exceeded",
                    slog.String("error_type", httpErr.ErrorType),
                    slog.String("error_message", httpErr.Message),
                    slog.String("model", config["model_name"].(string)),
                    slog.String("image_size", config["image_size"].(string)),
                    slog.Int("status_code", httpErr.StatusCode))
                return "", fmt.Errorf("OpenAI Image quota exceeded: %s (Type: %s)", httpErr.Message, httpErr.ErrorType)
            }

            s.logger.Error("OpenAI Image API error",
                slog.Int("attempt", attempt),
                slog.Int("status_code", httpErr.StatusCode),
                slog.String("error_type", httpErr.ErrorType),
                slog.String("error_message", httpErr.Message),
                slog.String("raw_body", httpErr.RawBody))
        }

        if attempt == maxRetries {
            s.logger.Error("Error calling OpenAI Image API after multiple attempts",
                slog.Int("attempts", maxRetries),
                slog.String("error", err.Error()),
                slog.String("model", config["model_name"].(string)))
            return "", fmt.Errorf("failed to call OpenAI Image API after %d attempts: %w", maxRetries, err)
        }

        s.logger.Warn("Attempt failed, retrying",
            slog.Int("attempt", attempt),
            slog.Duration("retry_delay", retryDelay),
            slog.String("error", err.Error()))

        time.Sleep(retryDelay)
    }

    return "", fmt.Errorf("failed to call OpenAI Image API after exhausting all retry attempts")
}

func (s *OpenAIImageService) callOpenAIImage(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
    apiURL, ok := config["api_url"].(string)
    if !ok {
        return "", fmt.Errorf("api_url not found in config")
    }

    apiKey, ok := config["api_key"].(string)
    if !ok {
        return "", fmt.Errorf("api_key not found in config")
    }

    modelName, ok := config["model_name"].(string)
    if !ok {
        return "", fmt.Errorf("model_name not found in config")
    }

    imageSize, ok := config["image_size"].(string)
    if !ok {
        imageSize = "1024x1024" // Default size
    }

    requestBody, err := json.Marshal(map[string]interface{}{
        "model": modelName,
        "prompt": prompt,
        "n": 1,
        "size": imageSize,
        "response_format": "url",
    })
    if err != nil {
        return "", fmt.Errorf("error marshaling request body: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(requestBody))
    if err != nil {
        return "", fmt.Errorf("error creating request: %w", err)
    }

    req.Header.Set("Authorization", "Bearer "+apiKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := s.httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("error making request: %w", err)
    }

    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        rawBody, openAIErr := extractOpenAIErrorDetails(resp)
        httpErr := &OpenAIHttpError{
            StatusCode: resp.StatusCode,
            RawBody:    rawBody,
        }

        if openAIErr != nil {
            httpErr.Message = openAIErr.Error.Message
            httpErr.ErrorType = openAIErr.Error.Type
        } else {
            httpErr.Message = "Unknown error"
            httpErr.ErrorType = "unknown"
        }

        return "", httpErr
    }





    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("error reading response body: %w", err)
    }

    var result map[string]interface{}
    if err := json.Unmarshal(body, &result); err != nil {
        return "", fmt.Errorf("error unmarshaling response: %w", err)
    }

    data, ok := result["data"].([]interface{})
    if !ok || len(data) == 0 {
        return "", fmt.Errorf("unexpected response format from OpenAI Image API")
    }

    imageURL, ok := data[0].(map[string]interface{})["url"].(string)
    if !ok {
        return "", fmt.Errorf("image URL not found in OpenAI Image API response")
    }

    // Return the image URL directly
    return imageURL, nil
}