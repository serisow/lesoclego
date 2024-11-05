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

type OpenAIService struct {
    httpClient *http.Client
    logger     *slog.Logger
}


func NewOpenAIService(logger *slog.Logger) *OpenAIService {
    return &OpenAIService{
        httpClient: &http.Client{Timeout: 120 * time.Second},
        logger:     logger,
    }
}

func (s *OpenAIService) CallLLM(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
    maxRetries := 3
    retryDelay := 5 * time.Second

    for attempt := 1; attempt <= maxRetries; attempt++ {

        response, err := s.callOpenAI(ctx, config, prompt)
        if err == nil {
            return response, nil
        }

        // Check if error contains OpenAI error details
        if httpErr, ok := err.(*OpenAIHttpError); ok {
            if httpErr.StatusCode == 429 {
                s.logger.Error("OpenAI API quota exceeded",
                    slog.String("error_type", httpErr.ErrorType),
                    slog.String("error_message", httpErr.Message),
                    slog.String("model", config["model_name"].(string)),
                    slog.Int("status_code", httpErr.StatusCode))
                return "", fmt.Errorf("OpenAI quota exceeded: %s (Type: %s)", httpErr.Message, httpErr.ErrorType)
            }

            s.logger.Error("OpenAI API error",
                slog.Int("attempt", attempt),
                slog.Int("status_code", httpErr.StatusCode),
                slog.String("error_type", httpErr.ErrorType),
                slog.String("error_message", httpErr.Message),
                slog.String("raw_body", httpErr.RawBody))
        }

        if attempt == maxRetries {
            s.logger.Error("Error calling OpenAI API after multiple attempts",
                slog.Int("attempts", maxRetries),
                slog.String("error", err.Error()),
                slog.String("model", config["model_name"].(string)))
            return "", fmt.Errorf("failed to call OpenAI API after %d attempts: %w", maxRetries, err)
        }

        s.logger.Warn("Attempt failed, retrying",
            slog.Int("attempt", attempt),
            slog.Duration("retry_delay", retryDelay),
            slog.String("error", err.Error()))

        time.Sleep(retryDelay)
    }

    return "", fmt.Errorf("failed to call OpenAI API after exhausting all retry attempts")
}

func (s *OpenAIService) callOpenAI(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
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

    messages := []map[string]string{
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": prompt},
    }

    requestBody, err := json.Marshal(map[string]interface{}{
        "model":    modelName,
        "messages": messages,
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

    choices, ok := result["choices"].([]interface{})
    if !ok || len(choices) == 0 {
        return "", fmt.Errorf("unexpected response format from OpenAI API")
    }

    firstChoice, ok := choices[0].(map[string]interface{})
    if !ok {
        return "", fmt.Errorf("unexpected choice format in OpenAI API response")
    }

    message, ok := firstChoice["message"].(map[string]interface{})
    if !ok {
        return "", fmt.Errorf("message not found in OpenAI API response")
    }

    content, ok := message["content"].(string)
    if !ok {
        return "", fmt.Errorf("content not found in OpenAI API response")
    }

    return content, nil
}



