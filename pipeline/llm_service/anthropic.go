package llm_service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"
)

type AnthropicService struct {
    httpClient *http.Client
    logger     *zap.Logger
}

func NewAnthropicService(logger *zap.Logger) *AnthropicService {
    return &AnthropicService{
        httpClient: &http.Client{Timeout: 120 * time.Second},
        logger:     logger,
    }
}

func (s *AnthropicService) CallLLM(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
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


    maxTokens, ok := config["parameters"].(map[string]interface{})["max_tokens"]
    if !ok {
        return "", fmt.Errorf("max_tokens not found in config parameters")
    }

    // Convert maxTokens to int, handling both string and float64 cases
    var maxTokensInt int
    switch v := maxTokens.(type) {
    case string:
        parsedValue, err := strconv.Atoi(v)
        if err != nil {
            return "", fmt.Errorf("failed to parse max_tokens as integer: %w", err)
        }
        maxTokensInt = parsedValue
    case float64:
        maxTokensInt = int(v)
    default:
        return "", fmt.Errorf("unexpected type for max_tokens: %T", maxTokens)
    }
    
    requestBody, err := json.Marshal(map[string]interface{}{
        "model": modelName,
        "messages": []map[string]string{
            {"role": "user", "content": prompt},
        },
        "max_tokens": maxTokensInt,
    })
    if err != nil {
        return "", fmt.Errorf("error marshaling request body: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(requestBody))
    if err != nil {
        return "", fmt.Errorf("error creating request: %w", err)
    }

    req.Header.Set("x-api-key", apiKey)
    req.Header.Set("anthropic-version", "2023-06-01")
    req.Header.Set("Content-Type", "application/json")

    resp, err := s.httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("error making request: %w", err)
    }
    defer resp.Body.Close()

    var result map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", fmt.Errorf("error decoding response: %w", err)
    }

    content, ok := result["content"].([]interface{})
    if !ok || len(content) == 0 {
        return "", fmt.Errorf("unexpected response format from Anthropic API")
    }

    message, ok := content[0].(map[string]interface{})
    if !ok {
        return "", fmt.Errorf("unexpected message format in Anthropic API response")
    }

    text, ok := message["text"].(string)
    if !ok {
        return "", fmt.Errorf("text not found in Anthropic API response")
    }

    return text, nil
}