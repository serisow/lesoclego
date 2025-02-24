package llm_service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type ElevenLabsService struct {
	httpClient *http.Client
	logger     *slog.Logger
}

// Voice settings structure matching the Drupal configuration
type VoiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style"`
	UseSpeakerBoost bool    `json:"use_speaker_boost"`
}

// Audio file response structure
type AudioFileResponse struct {
	FileID    string `json:"file_id"`
	URI       string `json:"uri"`
	URL       string `json:"url"`
	MimeType  string `json:"mime_type"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	Timestamp int64  `json:"timestamp"`
}

func NewElevenLabsService(logger *slog.Logger) *ElevenLabsService {
	return &ElevenLabsService{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		logger:     logger,
	}
}

func (s *ElevenLabsService) CallLLM(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
	maxRetries := 3
	retryDelay := 5 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		response, err := s.callElevenLabs(ctx, config, prompt)
		if err == nil {
			return response, nil
		}

		// Check if error contains ElevenLabs error details
		if httpErr, ok := err.(*ElevenLabsHttpError); ok {
			if httpErr.StatusCode == 429 {
				s.logger.Error("ElevenLabs API quota exceeded",
					slog.String("error_type", httpErr.ErrorType),
					slog.String("error_message", httpErr.Message),
					slog.String("model", config["model_name"].(string)),
					slog.Int("status_code", httpErr.StatusCode))
				return "", fmt.Errorf("ElevenLabs quota exceeded: %s (Type: %s)", httpErr.Message, httpErr.ErrorType)
			}

			s.logger.Error("ElevenLabs API error",
				slog.Int("attempt", attempt),
				slog.Int("status_code", httpErr.StatusCode),
				slog.String("error_type", httpErr.ErrorType),
				slog.String("error_message", httpErr.Message),
				slog.String("raw_body", httpErr.RawBody))
		}

		if attempt == maxRetries {
			s.logger.Error("Error calling ElevenLabs API after multiple attempts",
				slog.Int("attempts", maxRetries),
				slog.String("error", err.Error()),
				slog.String("model", config["model_name"].(string)))
			return "", fmt.Errorf("failed to call ElevenLabs API after %d attempts: %w", maxRetries, err)
		}

		s.logger.Warn("Attempt failed, retrying",
			slog.Int("attempt", attempt),
			slog.Duration("retry_delay", retryDelay),
			slog.String("error", err.Error()))

		time.Sleep(retryDelay)
	}

	return "", fmt.Errorf("failed to call ElevenLabs API after exhausting all retry attempts")
}

func (s *ElevenLabsService) callElevenLabs(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
	// Extract required configuration
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

	params, ok := config["parameters"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("parameters not found in config")
	}

	voiceID, ok := params["voice_id"].(string)
	if !ok {
		return "", fmt.Errorf("voice_id not found in parameters")
	}

	// Extract voice settings
	voiceSettings := VoiceSettings{
		Stability:       getFloat64(params, "stability", 0.5),
		SimilarityBoost: getFloat64(params, "similarity_boost", 0.75),
		Style:           getFloat64(params, "style", 0),
		UseSpeakerBoost: getBool(params, "use_speaker_boost", true),
	}

	// Prepare request body
	requestBody, err := json.Marshal(map[string]interface{}{
		"text":      prompt,
		"model_id":  modelName,
		"voice_settings": voiceSettings,
	})
	if err != nil {
		return "", fmt.Errorf("error marshaling request body: %w", err)
	}

	// Create request
	fullURL := fmt.Sprintf("%s/%s", apiURL, voiceID)
	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("xi-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

	// Execute request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	// Handle non-200 responses
	if resp.StatusCode != http.StatusOK {
		return "", s.handleErrorResponse(resp)
	}

	// Process successful response
	return s.processAudioResponse(resp)
}

func (s *ElevenLabsService) processAudioResponse(resp *http.Response) (string, error) {
	// Create directory structure
	directory := filepath.Join("storage", "pipeline", "audio", time.Now().Format("2006-01"))
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate unique filename
	filename := fmt.Sprintf("tts_%d.mp3", time.Now().UnixNano())
	filepath := filepath.Join(directory, filename)

	// Create file
	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create audio file: %w", err)
	}
	defer file.Close()

	// Copy audio data to file
	written, err := io.Copy(file, resp.Body)
	if err != nil {
		os.Remove(filepath) // Clean up on error
		return "", fmt.Errorf("failed to write audio data: %w", err)
	}

	// Prepare response
	_, err = file.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}

	response := AudioFileResponse{
		FileID:    fmt.Sprintf("%d", time.Now().UnixNano()),
		URI:       filepath,
		URL:       fmt.Sprintf("/storage/pipeline/audio/%s/%s", time.Now().Format("2006-01"), filename),
		MimeType:  "audio/mpeg",
		Filename:  filename,
		Size:      written,
		Timestamp: time.Now().Unix(),
	}

	// Convert to JSON
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(jsonResponse), nil
}

func (s *ElevenLabsService) handleErrorResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &ElevenLabsHttpError{
			StatusCode: resp.StatusCode,
			Message:    "Failed to read error response",
			ErrorType:  "unknown",
			RawBody:    "",
		}
	}

	var errorResp struct {
		Detail struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"detail"`
	}

	if err := json.Unmarshal(body, &errorResp); err != nil {
		return &ElevenLabsHttpError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
			ErrorType:  "unknown",
			RawBody:    string(body),
		}
	}

	return &ElevenLabsHttpError{
		StatusCode: resp.StatusCode,
		Message:    errorResp.Detail.Message,
		ErrorType:  errorResp.Detail.Status,
		RawBody:    string(body),
	}
}

// Helper functions
func getFloat64(params map[string]interface{}, key string, defaultValue float64) float64 {
	if val, ok := params[key].(float64); ok {
		return val
	}
	return defaultValue
}

func getBool(params map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := params[key].(bool); ok {
		return val
	}
	return defaultValue
}

type ElevenLabsHttpError struct {
	StatusCode int
	Message    string
	ErrorType  string
	RawBody    string
}

func (e *ElevenLabsHttpError) Error() string {
	return fmt.Sprintf("ElevenLabs API error (HTTP %d): %s (Type: %s)", e.StatusCode, e.Message, e.ErrorType)
}