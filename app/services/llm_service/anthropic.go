// anthropic.go

package llm_service

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
    "log/slog"
)

type AnthropicService struct {
    httpClient *http.Client
    logger     *slog.Logger
}

func NewAnthropicService(logger *slog.Logger) *AnthropicService {
    return &AnthropicService{
        httpClient: &http.Client{Timeout: 120 * time.Second},
        logger:     logger,
    }
}

func (s *AnthropicService) CallLLM(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
    maxRetries := 3
    retryDelay := 5 * time.Second

    for attempt := 1; attempt <= maxRetries; attempt++ {
        response, err := s.callAnthropic(ctx, config, prompt)
        if err == nil {
            return response, nil
        }

        if attempt == maxRetries {
            s.logger.Error("Error calling Anthropic API after multiple attempts",
                slog.Int("attempts", maxRetries),
                slog.String("error", err.Error()))
            return "", fmt.Errorf("failed to call Anthropic API after %d attempts: %w", maxRetries, err)
        }

        s.logger.Warn("Attempt failed, retrying",
            slog.Int("attempt", attempt),
            slog.Duration("retryDelay", retryDelay),
            slog.String("error", err.Error()))
        time.Sleep(retryDelay)
    }

    return "", fmt.Errorf("failed to call Anthropic API after exhausting all retry attempts")
}

func (s *AnthropicService) callAnthropic(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
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

    maxTokensInt := int(safeParseFloat(maxTokens, 1000))

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
