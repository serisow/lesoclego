package action_service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/serisow/lesocle/pipeline_type"
)

const (
    LinkedInShareServiceName = "linkedin_share"
    linkedInAPIBaseURL      = "https://api.linkedin.com/v2"
)

type LinkedInShareActionService struct {
    logger *slog.Logger
}

func NewLinkedInShareActionService(logger *slog.Logger) *LinkedInShareActionService {
    return &LinkedInShareActionService{
        logger: logger,
    }
}

type LinkedInCredentials struct {
    AccessToken string
    AuthorID    string
}

type LinkedInContent struct {
    Text  string       `json:"text"`
    Media *MediaContent `json:"media,omitempty"`
}

type MediaContent struct {
    URL         string `json:"url"`
    Title       string `json:"title"`
    Description string `json:"description"`
    Thumbnail   string `json:"thumbnail,omitempty"`
}

func (s *LinkedInShareActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
        return "", fmt.Errorf("missing action configuration for LinkedInShareAction")
    }

    config := step.ActionDetails.Configuration
    credentials, err := extractLinkedInCredentials(config)
    if err != nil {
        return "", fmt.Errorf("error extracting LinkedIn credentials: %w", err)
    }

    // Get content from required steps
    var content string
    requiredSteps := strings.Split(step.RequiredSteps, "\r\n")
    for _, requiredStep := range requiredSteps {
        requiredStep = strings.TrimSpace(requiredStep)
        if requiredStep == "" {
            continue
        }

        // Get the step output
        stepOutput, ok := pipelineContext.GetStepOutput(requiredStep)
        if !ok {
            return "", fmt.Errorf("required step output '%s' not found for LinkedIn content", requiredStep)
        }

        // Try to detect if this is from a social media step type
        var resultData map[string]interface{}
        if err := json.Unmarshal([]byte(fmt.Sprintf("%v", stepOutput)), &resultData); err == nil {
            if platforms, ok := resultData["platforms"].(map[string]interface{}); ok {
                if linkedinContent, ok := platforms["linkedin"].(map[string]interface{}); ok {
                    // This is from a social media step, use the linkedin content
                    linkedinJSON, err := json.Marshal(linkedinContent)
                    if err != nil {
                        return "", fmt.Errorf("error marshaling linkedin content: %w", err)
                    }
                    content = string(linkedinJSON)
                    break
                }
            }
        }
        
        // If not from social media step, use content as is (existing behavior)
        content += fmt.Sprintf("%v", stepOutput)
    }

    if content == "" {
        s.logger.Error("LinkedIn content is empty",
            slog.String("step_id", step.ID),
            slog.String("required_steps", step.RequiredSteps))
        return "", fmt.Errorf("LinkedIn content is empty")
    }

    // Parse and validate the content
    linkedInContent, err := s.parseAndValidateContent(content)
    if err != nil {
        return "", fmt.Errorf("error parsing LinkedIn content: %w", err)
    }

    // Build the share payload
    payload := s.buildSharePayload(linkedInContent, credentials)

    // Create HTTP request
    url := fmt.Sprintf("%s/ugcPosts", linkedInAPIBaseURL)
    jsonData, err := json.Marshal(payload)
    if err != nil {
        return "", fmt.Errorf("error marshaling payload: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
    if err != nil {
        return "", fmt.Errorf("error creating request: %w", err)
    }

    // Set headers
    req.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

    // Execute request
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("error executing request: %w", err)
    }
    defer resp.Body.Close()

    // Handle response
    if resp.StatusCode == 201 {
        var createResponse struct {
            ID string `json:"id"`
        }
        if err := json.NewDecoder(resp.Body).Decode(&createResponse); err != nil {
            return "", fmt.Errorf("error decoding response: %w", err)
        }

        result := map[string]interface{}{
            "post_id": createResponse.ID,
            "text":    linkedInContent.Text,
            "type":    getContentType(linkedInContent),
        }

        resultJSON, err := json.Marshal(result)
        if err != nil {
            return "", fmt.Errorf("error marshaling result: %w", err)
        }

        return string(resultJSON), nil
    }

    // Handle error response
    var errorResp struct {
        Message string `json:"message"`
        Status  int    `json:"status"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
        return "", fmt.Errorf("LinkedIn API error (status %d)", resp.StatusCode)
    }

    s.logger.Error("LinkedIn API error",
        slog.String("error", errorResp.Message),
        slog.Int("status_code", resp.StatusCode))

    return "", fmt.Errorf("LinkedIn API Error: %s", errorResp.Message)
}

func (s *LinkedInShareActionService) CanHandle(actionService string) bool {
    return actionService == LinkedInShareServiceName
}

func extractLinkedInCredentials(config map[string]interface{}) (*LinkedInCredentials, error) {
    credentials := &LinkedInCredentials{}
    var ok bool

    if credentials.AccessToken, ok = config["access_token"].(string); !ok {
        return nil, fmt.Errorf("access_token not found in config")
    }
    if credentials.AuthorID, ok = config["author_id"].(string); !ok {
        return nil, fmt.Errorf("author_id not found in config")
    }

    return credentials, nil
}

func (s *LinkedInShareActionService) parseAndValidateContent(content string) (*LinkedInContent, error) {
    // Remove JSON code block markers if present
    content = cleanJsonContent(content)

    var linkedInContent LinkedInContent
    if err := json.Unmarshal([]byte(content), &linkedInContent); err != nil {
        return nil, fmt.Errorf("invalid JSON format: %w", err)
    }

    // Validate required text field
    if linkedInContent.Text == "" {
        return nil, fmt.Errorf("JSON must contain a non-empty 'text' field")
    }

    // Validate media content if present
    if linkedInContent.Media != nil {
        if linkedInContent.Media.URL == "" {
            return nil, fmt.Errorf("media content must include 'url' field")
        }
        if linkedInContent.Media.Title == "" {
            return nil, fmt.Errorf("media content must include 'title' field")
        }
        if linkedInContent.Media.Description == "" {
            return nil, fmt.Errorf("media content must include 'description' field")
        }
    }

    return &linkedInContent, nil
}

func (s *LinkedInShareActionService) buildSharePayload(content *LinkedInContent, credentials *LinkedInCredentials) map[string]interface{} {
    payload := map[string]interface{}{
        "author":         credentials.AuthorID,
        "lifecycleState": "PUBLISHED",
        "specificContent": map[string]interface{}{
            "com.linkedin.ugc.ShareContent": map[string]interface{}{
                "shareCommentary": map[string]interface{}{
                    "text": content.Text,
                },
            },
        },
        "visibility": map[string]interface{}{
            "com.linkedin.ugc.MemberNetworkVisibility": "PUBLIC",
        },
    }

    shareContent := payload["specificContent"].(map[string]interface{})["com.linkedin.ugc.ShareContent"].(map[string]interface{})

    if content.Media != nil {
        shareContent["shareMediaCategory"] = "ARTICLE"
        shareContent["media"] = []map[string]interface{}{
            {
                "status":      "READY",
                "originalUrl": content.Media.URL,
                "title": map[string]interface{}{
                    "text": content.Media.Title,
                },
                "description": map[string]interface{}{
                    "text": content.Media.Description,
                },
            },
        }

        if content.Media.Thumbnail != "" {
            shareContent["media"].([]map[string]interface{})[0]["thumbnails"] = []map[string]interface{}{
                {"url": content.Media.Thumbnail},
            }
        }
    } else {
        shareContent["shareMediaCategory"] = "NONE"
    }

    return payload
}

func getContentType(content *LinkedInContent) string {
    if content.Media != nil {
        return "article"
    }
    return "text"
}