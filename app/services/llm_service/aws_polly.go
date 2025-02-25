package llm_service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/polly"
)

type AWSPollyService struct {
	logger *slog.Logger
}

// Audio file response structure to match what Drupal expects
type PollyAudioFileResponse struct {
	FileID    string `json:"file_id"`
	URI       string `json:"uri"`
	URL       string `json:"url"`
	MimeType  string `json:"mime_type"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	Timestamp int64  `json:"timestamp"`
}

func NewAWSPollyService(logger *slog.Logger) *AWSPollyService {
	return &AWSPollyService{
		logger: logger,
	}
}

func (s *AWSPollyService) CallLLM(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
	maxRetries := 3
	retryDelay := 5 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		response, err := s.callAWSPolly(ctx, config, prompt)
		if err == nil {
			return response, nil
		}

		if attempt == maxRetries {
			s.logger.Error("Error calling AWS Polly API after multiple attempts",
				slog.Int("attempts", maxRetries),
				slog.String("error", err.Error()))
			return "", fmt.Errorf("failed to call AWS Polly API after %d attempts: %w", maxRetries, err)
		}

		s.logger.Warn("Attempt failed, retrying",
			slog.Int("attempt", attempt),
			slog.Duration("retry_delay", retryDelay),
			slog.String("error", err.Error()))

		time.Sleep(retryDelay)
	}

	return "", fmt.Errorf("failed to call AWS Polly API after exhausting all retry attempts")
}

func (s *AWSPollyService) callAWSPolly(ctx context.Context, config map[string]interface{}, prompt string) (string, error) {
	// Extract required configuration
	apiKey, ok := config["api_key"].(string)
	if !ok {
		return "", fmt.Errorf("api_key not found in config")
	}

	// Get API secret from environment variable as requested
	apiSecret := os.Getenv("AWS_API_SECRET")
	if apiSecret == "" {
		return "", fmt.Errorf("AWS_API_SECRET environment variable is not set")
	}

	params, ok := config["parameters"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("parameters not found in config")
	}

	// Get required parameters with fallbacks
	region := getStringParam(params, "region", "us-west-2")
	voiceId := getStringParam(params, "voice_id", "Joanna")
	outputFormat := getStringParam(params, "output_format", "mp3")
	sampleRate := getStringParam(params, "sample_rate", "22050")
	engine := getStringParam(params, "engine", "standard")

	// Create AWS session
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(apiKey, apiSecret, ""),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create AWS session: %w", err)
	}

	// Create Polly client
	pollyClient := polly.New(sess)

	// Create synthesize speech input
	input := &polly.SynthesizeSpeechInput{
		Text:         aws.String(prompt),
		OutputFormat: aws.String(outputFormat),
		VoiceId:      aws.String(voiceId),
		Engine:       aws.String(engine),
		SampleRate:   aws.String(sampleRate),
	}

	// Call SynthesizeSpeech API
	output, err := pollyClient.SynthesizeSpeechWithContext(ctx, input)
	if err != nil {
		return "", fmt.Errorf("error calling AWS Polly SynthesizeSpeech: %w", err)
	}
	defer output.AudioStream.Close()

	// Create directory structure
	directory := filepath.Join("storage", "pipeline", "audio", time.Now().Format("2006-01"))
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate unique filename
	filename := fmt.Sprintf("polly_%d.%s", time.Now().UnixNano(), outputFormat)
	filepath := filepath.Join(directory, filename)

	// Create file
	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create audio file: %w", err)
	}
	defer file.Close()

	// Copy audio data to file
	written, err := io.Copy(file, output.AudioStream)
	if err != nil {
		os.Remove(filepath) // Clean up on error
		return "", fmt.Errorf("failed to write audio data: %w", err)
	}

	// Prepare response
	response := PollyAudioFileResponse{
		FileID:    fmt.Sprintf("%d", time.Now().UnixNano()),
		URI:       filepath,
		URL:       fmt.Sprintf("/storage/pipeline/audio/%s/%s", time.Now().Format("2006-01"), filename),
		MimeType:  fmt.Sprintf("audio/%s", outputFormat),
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

// Helper function to get string parameters with defaults
func getStringParam(params map[string]interface{}, key, defaultValue string) string {
	if val, ok := params[key].(string); ok && val != "" {
		return val
	}
	return defaultValue
}