package llm_service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
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

    for attempt := 1; attempt <= maxRetries; attempt++ {
        response, err := s.callGemini(ctx, config, prompt)
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

   // modelName, ok := config["model_name"].(string)
    if !ok {
        return "", fmt.Errorf("model_name not found in config")
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
            "topK":             safeParseFloat(params["top_k"], 64.0),
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