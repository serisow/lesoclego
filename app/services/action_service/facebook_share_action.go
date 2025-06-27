package action_service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/serisow/lesocle/pipeline_type"
)

const (
	FacebookShareServiceName = "facebook_share"
	facebookAPIBaseURL       = "https://graph.facebook.com"
)

type FacebookShareActionService struct {
	logger *slog.Logger
}

func NewFacebookShareActionService(logger *slog.Logger) *FacebookShareActionService {
	return &FacebookShareActionService{
		logger: logger,
	}
}

type FacebookCredentials struct {
	AccessToken string
	PageID      string
	APIVersion  string
}

func (s *FacebookShareActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
	if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
		return "", fmt.Errorf("missing action configuration for FacebookShareAction")
	}

	config := step.ActionDetails.Configuration
	credentials, err := extractFacebookCredentials(config)
	if err != nil {
		return "", fmt.Errorf("error extracting Facebook credentials: %w", err)
	}

	// Validate token and page access before proceeding
	if err := s.validateAccessToken(ctx, credentials); err != nil {
		return "", fmt.Errorf("token validation failed: %w", err)
	}

	// Find Facebook content in the context
	facebookContent := s.findFacebookContent(step, pipelineContext)
	if facebookContent == "" {
		s.logger.Error("Facebook content is empty",
			slog.String("step_id", step.ID),
			slog.String("required_steps", step.RequiredSteps))
		return "", fmt.Errorf("facebook content is empty")
	}

	// Parse and validate the content
	data, err := s.parseAndValidateContent(facebookContent)
	if err != nil {
		return "", err
	}

	// Note: For production, ensure image URLs are properly configured
	data.ImageURL = "https://i.postimg.cc/Y0jyFx5m/test-sharing-image.webp"
	// Choose posting method based on content type
	if data.ImageURL != "" {
		return s.postPhoto(ctx, data, credentials)
	}
	return s.postLink(ctx, data, credentials)
}

func (s *FacebookShareActionService) validateAccessToken(ctx context.Context, credentials *FacebookCredentials) error {
	facebookUrl := fmt.Sprintf("%s/%s/%s",
		facebookAPIBaseURL,
		credentials.APIVersion,
		credentials.PageID)

	req, err := http.NewRequestWithContext(ctx, "GET", facebookUrl, nil)
	if err != nil {
		return fmt.Errorf("error creating validation request: %w", err)
	}

	q := req.URL.Query()
	q.Add("access_token", credentials.AccessToken)
	q.Add("fields", "id,name")
	req.URL.RawQuery = q.Encode()

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error validating token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return s.handleErrorResponse(resp)
	}

	var result struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("error decoding validation response: %w", err)
	}

	if result.ID != credentials.PageID {
		return fmt.Errorf("invalid page access: token does not match page ID")
	}

	return nil
}

func (s *FacebookShareActionService) findFacebookContent(step *pipeline_type.PipelineStep, pipelineContext *pipeline_type.Context) string {
	// First try to get content from a social media step
	var content string
	requiredSteps := strings.Split(step.RequiredSteps, "\r\n")

	for _, requiredStep := range requiredSteps {
		requiredStep = strings.TrimSpace(requiredStep)
		if requiredStep == "" {
			continue
		}

		if stepOutput, ok := pipelineContext.GetStepOutput(requiredStep); ok {
			// Try to parse as social media step output
			var resultData map[string]interface{}
			if err := json.Unmarshal([]byte(fmt.Sprintf("%v", stepOutput)), &resultData); err == nil {
				if platforms, ok := resultData["platforms"].(map[string]interface{}); ok {
					if facebookContent, ok := platforms["facebook"].(map[string]interface{}); ok {
						// This is from a social media step, use the facebook content
						if contentJSON, err := json.Marshal(facebookContent); err == nil {
							return string(contentJSON)
						}
					}
				}
			}
			content += fmt.Sprintf("%v", stepOutput)
		}
	}

	return content
}

func (s *FacebookShareActionService) parseAndValidateContent(content string) (*struct {
	Text     string `json:"text"`
	URL      string `json:"url,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}, error) {
	// Remove JSON code block markers if present
	content = cleanJsonContent(content)

	var data struct {
		Text     string `json:"text"`
		URL      string `json:"url,omitempty"`
		ImageURL string `json:"image_url,omitempty"`
	}

	if err := json.Unmarshal([]byte(content), &data); err != nil {
		return nil, fmt.Errorf("invalid JSON format: %w", err)
	}

	// Validate required fields
	if data.Text == "" {
		return nil, fmt.Errorf("text field is required")
	}

	if data.URL == "" && data.ImageURL == "" {
		return nil, fmt.Errorf("either url or image_url field is required")
	}

	return &data, nil
}

func (s *FacebookShareActionService) postLink(ctx context.Context, data *struct {
	Text     string `json:"text"`
	URL      string `json:"url,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}, credentials *FacebookCredentials) (string, error) {
	facebookUrl := fmt.Sprintf("%s/%s/%s/feed",
		facebookAPIBaseURL,
		credentials.APIVersion,
		credentials.PageID)

	// Create form data
	formData := url.Values{}
	formData.Set("message", data.Text)
	formData.Set("link", data.URL)
	formData.Set("access_token", credentials.AccessToken)

	// Make request with form data
	req, err := http.NewRequestWithContext(ctx, "POST", facebookUrl, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
			return "", fmt.Errorf("facebook API error (HTTP %d)", resp.StatusCode)
		}
		return "", fmt.Errorf("facebook API error: %s (Type: %s, Code: %d)",
			errorResp.Error.Message,
			errorResp.Error.Type,
			errorResp.Error.Code)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	response := map[string]interface{}{
		"post_id": result.ID,
		"text":    data.Text,
		"type":    "link",
	}

	resultJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("error marshaling result: %w", err)
	}

	return string(resultJSON), nil
}

func (s *FacebookShareActionService) postPhoto(ctx context.Context, data *struct {
	Text     string `json:"text"`
	URL      string `json:"url,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}, credentials *FacebookCredentials) (string, error) {
	// First validate the image URL is accessible
	err := s.validateImageURL(ctx, data.ImageURL)
	if err != nil {
		s.logger.Warn("Image validation failed, falling back to link post",
			slog.String("error", err.Error()),
			slog.String("image_url", data.ImageURL))

		// If we have a URL, fall back to link post
		if data.URL != "" {
			return s.postLink(ctx, data, credentials)
		}
		return "", fmt.Errorf("unable to post content: invalid image URL and no fallback URL available")
	}

	facebookUrl := fmt.Sprintf("%s/%s/%s/photos",
		facebookAPIBaseURL,
		credentials.APIVersion,
		credentials.PageID)

	// Create form data - this is key for Facebook's API
	formData := url.Values{}
	formData.Set("message", data.Text)
	formData.Set("url", data.ImageURL)
	formData.Set("access_token", credentials.AccessToken)

	// Make request with form data
	req, err := http.NewRequestWithContext(ctx, "POST", facebookUrl, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
			return "", fmt.Errorf("facebook API error (HTTP %d)", resp.StatusCode)
		}
		return "", fmt.Errorf("facebook API error: %s (Type: %s, Code: %d)",
			errorResp.Error.Message,
			errorResp.Error.Type,
			errorResp.Error.Code)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	response := map[string]interface{}{
		"post_id": result.ID,
		"text":    data.Text,
		"type":    "photo",
	}

	resultJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("error marshaling result: %w", err)
	}

	return string(resultJSON), nil
}

func (s *FacebookShareActionService) validateImageURL(ctx context.Context, imageURL string) error {
	if !strings.HasPrefix(imageURL, "http") {
		return fmt.Errorf("invalid image URL format: must start with http/https")
	}

	// Parse the URL to ensure it's valid
	parsedURL, err := url.Parse(imageURL)
	if err != nil {
		return fmt.Errorf("invalid image URL: %w", err)
	}

	// Check for common image extensions
	validExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}
	hasValidExtension := false
	lowercasePath := strings.ToLower(parsedURL.Path)
	for _, ext := range validExtensions {
		if strings.HasSuffix(lowercasePath, ext) {
			hasValidExtension = true
			break
		}
	}

	// If no valid extension, try to validate via HEAD request
	if !hasValidExtension {
		req, err := http.NewRequestWithContext(ctx, "HEAD", imageURL, nil)
		if err != nil {
			return fmt.Errorf("error creating image validation request: %w", err)
		}

		client := &http.Client{
			Timeout: 10 * time.Second,
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error validating image URL: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("image URL returned status %d", resp.StatusCode)
		}

		contentType := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			return fmt.Errorf("URL does not point to an image (content-type: %s)", contentType)
		}
	}

	return nil
}

func (s *FacebookShareActionService) handleErrorResponse(resp *http.Response) error {
	var errorResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		} `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
		return fmt.Errorf("facebook API error (HTTP %d)", resp.StatusCode)
	}

	s.logger.Error("Facebook API error",
		slog.String("message", errorResp.Error.Message),
		slog.String("type", errorResp.Error.Type),
		slog.Int("code", errorResp.Error.Code))

	return fmt.Errorf("facebook API error: %s (Type: %s, Code: %d)",
		errorResp.Error.Message,
		errorResp.Error.Type,
		errorResp.Error.Code)
}

func extractFacebookCredentials(config map[string]interface{}) (*FacebookCredentials, error) {
	credentials := &FacebookCredentials{}
	var ok bool

	if credentials.AccessToken, ok = config["access_token"].(string); !ok {
		return nil, fmt.Errorf("access_token not found in config")
	}
	if credentials.PageID, ok = config["page_id"].(string); !ok {
		return nil, fmt.Errorf("page_id not found in config")
	}
	if credentials.APIVersion, ok = config["api_version"].(string); !ok {
		credentials.APIVersion = "v22.0" // Default version
	}

	return credentials, nil
}

func extractDomainFromURL(urlStr string) string {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}

	// Return the scheme + host as the base URL
	if parsedURL.Host != "" {
		return fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
	}
	return ""
}

func (s *FacebookShareActionService) CanHandle(actionService string) bool {
	return actionService == FacebookShareServiceName
}
