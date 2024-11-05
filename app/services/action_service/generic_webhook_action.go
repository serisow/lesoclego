package action_service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/serisow/lesocle/pipeline_type"
)

const GenericWebhookServiceName = "generic_webhook"

type WebhookConfig struct {
	WebhookURL       string            `json:"webhook_url"`
	HTTPMethod       string            `json:"http_method"`
	Timeout          int               `json:"timeout"`
	RetryAttempts    int               `json:"retry_attempts"`
	CustomHeaders    map[string]string `json:"custom_headers"`
	Authentication   string            `json:"authentication"`
	Username         string            `json:"username,omitempty"`
	Password         string            `json:"password,omitempty"`
	Token            string            `json:"token,omitempty"`
	HeaderName       string            `json:"header_name,omitempty"`
	HeaderValue      string            `json:"header_value,omitempty"`
}


type GenericWebhookActionService struct {
	logger *slog.Logger
}

func NewGenericWebhookActionService(logger *slog.Logger) *GenericWebhookActionService {
	return &GenericWebhookActionService{
		logger: logger,
	}
}

func (s *GenericWebhookActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
	if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
		return "", fmt.Errorf("missing action configuration for GenericWebhookAction")
	}

	config := step.ActionDetails.Configuration
	credentials, err := extractWebhookConfig(config)
	if err != nil {
		return "", fmt.Errorf("error extracting webhook configuration: %w", err)
	}

	// Get content from required steps
	requiredSteps := strings.Split(step.RequiredSteps, "\r\n")
	var payloadContent string
	
	for _, requiredStep := range requiredSteps {
		requiredStep = strings.TrimSpace(requiredStep)
		if requiredStep == "" {
			continue
		}
		
		stepOutput, ok := pipelineContext.GetStepOutput(requiredStep)
		if !ok {
			return "", fmt.Errorf("required step output '%s' not found for webhook content", requiredStep)
		}
		payloadContent += fmt.Sprintf("%v", stepOutput)
	}

	if payloadContent == "" {
		s.logger.Error("Webhook content is empty",
			slog.String("step_id", step.ID),
			slog.String("required_steps", step.RequiredSteps))
		return "", fmt.Errorf("webhook content is empty")
	}

	// Prepare webhook payload
	payload := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"data":      payloadContent,
	}

	// Send webhook with retries
	result, err := s.sendWebhookWithRetry(ctx, credentials, payload)
	if err != nil {
		s.logger.Error("Failed to send webhook",
			slog.String("error", err.Error()),
			slog.String("webhook_url", credentials.WebhookURL))
		return "", fmt.Errorf("failed to send webhook: %w", err)
	}

	// Prepare response
	response := map[string]interface{}{
		"success":   true,
		"timestamp": time.Now().Unix(),
		"response":  result,
	}

	resultJson, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("error marshaling result: %w", err)
	}

	return string(resultJson), nil
}

func (s *GenericWebhookActionService) CanHandle(actionService string) bool {
	return actionService == GenericWebhookServiceName
}

func extractWebhookConfig(config map[string]interface{}) (*WebhookConfig, error) {
	webhookURL, ok := config["webhook_url"].(string)
	if !ok || webhookURL == "" {
		return nil, fmt.Errorf("webhook_url not found or empty in config")
	}

	// Extract and validate other configuration fields
	wc := &WebhookConfig{
		WebhookURL:    webhookURL,
		HTTPMethod:    getStringValue(config, "http_method", "POST"),
		Timeout:       getIntValue(config, "timeout", 30),
		RetryAttempts: getIntValue(config, "retry_attempts", 3),
		Authentication: getStringValue(config, "authentication", "none"),
	}

	// Parse custom headers if present
	if customHeadersStr, ok := config["custom_headers"].(string); ok && customHeadersStr != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(customHeadersStr), &headers); err != nil {
			return nil, fmt.Errorf("invalid custom headers JSON: %w", err)
		}
		wc.CustomHeaders = headers
	}

	// Extract authentication details based on type
	switch wc.Authentication {
	case "basic":
		wc.Username = getStringValue(config, "username", "")
		wc.Password = getStringValue(config, "password", "")
		if wc.Username == "" || wc.Password == "" {
			return nil, fmt.Errorf("username and password required for basic auth")
		}
	case "bearer":
		wc.Token = getStringValue(config, "token", "")
		if wc.Token == "" {
			return nil, fmt.Errorf("token required for bearer auth")
		}
	case "custom":
		wc.HeaderName = getStringValue(config, "header_name", "")
		wc.HeaderValue = getStringValue(config, "header_value", "")
		if wc.HeaderName == "" || wc.HeaderValue == "" {
			return nil, fmt.Errorf("header name and value required for custom auth")
		}
	}

	return wc, nil
}

func (s *GenericWebhookActionService) sendWebhookWithRetry(ctx context.Context, config *WebhookConfig, payload interface{}) (string, error) {
	var lastErr error
	
	for attempt := 0; attempt < config.RetryAttempts; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			jitter := time.Duration(float64(backoff) * (0.1 * (float64(time.Now().UnixNano()%100) / 100.0)))
			time.Sleep(backoff + jitter)
		}

		result, err := s.sendWebhook(ctx, config, payload)
		if err == nil {
			return result, nil
		}
		
		lastErr = err
		s.logger.Warn("Webhook attempt failed",
			slog.Int("attempt", attempt+1),
			slog.String("error", err.Error()))
	}

	return "", fmt.Errorf("all webhook attempts failed: %w", lastErr)
}

func (s *GenericWebhookActionService) sendWebhook(ctx context.Context, config *WebhookConfig, payload interface{}) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("error marshaling payload: %w", err)
	}

	// Create request with context and timeout
	ctx, cancel := context.WithTimeout(ctx, time.Duration(config.Timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, config.HTTPMethod, config.WebhookURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	
	// Add authentication headers
	switch config.Authentication {
	case "basic":
		req.SetBasicAuth(config.Username, config.Password)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+config.Token)
	case "custom":
		req.Header.Set(config.HeaderName, config.HeaderValue)
	}

	// Add custom headers
	for key, value := range config.CustomHeaders {
		req.Header.Set(key, value)
	}

	// Send request
	client := &http.Client{Timeout: time.Duration(config.Timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("webhook returned non-success status: %d", resp.StatusCode)
	}

	// Return response body as string
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("error marshaling response: %w", err)
	}

	return string(resultBytes), nil
}

// Helper functions
func getStringValue(config map[string]interface{}, key, defaultValue string) string {
	if val, ok := config[key].(string); ok {
		return val
	}
	return defaultValue
}

func getIntValue(config map[string]interface{}, key string, defaultValue int) int {
	if val, ok := config[key].(float64); ok {
		return int(val)
	}
	if val, ok := config[key].(int); ok {
		return val
	}
	return defaultValue
}