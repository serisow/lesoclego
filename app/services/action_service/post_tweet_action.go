package action_service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/dghubble/oauth1"
	"github.com/serisow/lesocle/pipeline_type"
)

const (
    PostTweetServiceName = "post_tweet"
    twitterAPIV2URL     = "https://api.twitter.com/2/tweets"
)

type PostTweetActionService struct {
    logger *slog.Logger
}

func NewPostTweetActionService(logger *slog.Logger) *PostTweetActionService {
    return &PostTweetActionService{
        logger: logger,
    }
}

func (s *PostTweetActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
    if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
        return "", fmt.Errorf("missing action configuration for PostTweetAction")
    }

    config := step.ActionDetails.Configuration
    credentials, err := extractTwitterCredentials(config)
    if err != nil {
        return "", fmt.Errorf("error extracting Twitter credentials: %w", err)
    }

    // Get content from required steps exactly like create_article_action
    requiredSteps := strings.Split(step.RequiredSteps, "\r\n")
    var content string
    
    for _, requiredStep := range requiredSteps {
        requiredStep = strings.TrimSpace(requiredStep)
        if requiredStep == "" {
            continue
        }
        
        stepOutput, ok := pipelineContext.GetStepOutput(requiredStep)
        if !ok {
            return "", fmt.Errorf("required step output '%s' not found for tweet content", requiredStep)
        }


        // Try to detect if this is from a social media step
        var resultData map[string]interface{}
        if err := json.Unmarshal([]byte(fmt.Sprintf("%v", stepOutput)), &resultData); err == nil {
            if platforms, ok := resultData["platforms"].(map[string]interface{}); ok {
                if twitterContent, ok := platforms["twitter"].(map[string]interface{}); ok {
                    // This is from a social media step, use the twitter content
                    twitterJSON, err := json.Marshal(twitterContent)
                    if err != nil {
                        return "", fmt.Errorf("error marshaling twitter content: %w", err)
                    }
                    content = string(twitterJSON)
                    break
                }
            }
        }

        content += fmt.Sprintf("%v", stepOutput)
    }

    if content == "" {
        s.logger.Error("Tweet content is empty",
            slog.String("step_id", step.ID),
            slog.String("required_steps", step.RequiredSteps))
        return "", fmt.Errorf("tweet content is empty")
    }

    // Clean and parse the JSON content
    tweetContent := cleanJsonContent(content)
    var tweetData struct {
        Text string `json:"text"`
    }
    if err := json.Unmarshal([]byte(tweetContent), &tweetData); err != nil {
        return "", fmt.Errorf("error parsing tweet content: %w", err)
    }

    if tweetData.Text == "" {
        return "", fmt.Errorf("JSON must contain 'text' field")
    }

    // Configure OAuth1.0a client
    oauthConfig := oauth1.NewConfig(credentials.ConsumerKey, credentials.ConsumerSecret)
    token := oauth1.NewToken(credentials.AccessToken, credentials.AccessTokenSecret)
    httpClient := oauthConfig.Client(ctx, token)

    // Prepare tweet payload
    tweetRequest := map[string]string{
        "text": tweetData.Text,
    }
    
    jsonData, err := json.Marshal(tweetRequest)
    if err != nil {
        return "", fmt.Errorf("error marshaling tweet request: %w", err)
    }

    // Create HTTP request
    req, err := http.NewRequestWithContext(ctx, "POST", twitterAPIV2URL, bytes.NewBuffer(jsonData))
    if err != nil {
        return "", fmt.Errorf("error creating request: %w", err)
    }
    req.Header.Set("Content-Type", "application/json")

    // Execute request
    resp, err := httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("error executing request: %w", err)
    }
    defer resp.Body.Close()

    // Parse response
    if resp.StatusCode == 201 {
        var tweetResponse struct {
            Data struct {
                ID   string `json:"id"`
                Text string `json:"text"`
            } `json:"data"`
        }

        if err := json.NewDecoder(resp.Body).Decode(&tweetResponse); err != nil {
            return "", fmt.Errorf("error decoding response: %w", err)
        }

        result := map[string]interface{}{
            "tweet_id": tweetResponse.Data.ID,
            "text":     tweetData.Text,
        }
        
        resultJson, err := json.Marshal(result)
        if err != nil {
            return "", fmt.Errorf("error marshaling result: %w", err)
        }

        return string(resultJson), nil
    }

    // Handle error case
    var errorResp struct {
        Errors []struct {
            Message string `json:"message"`
        } `json:"errors"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
        return "", fmt.Errorf("twitter API error (status %d)", resp.StatusCode)
    }

    errorMessage := "Unknown Twitter API error"
    if len(errorResp.Errors) > 0 {
        errorMessage = errorResp.Errors[0].Message
    }

    s.logger.Error("Twitter API error",
        slog.String("error", errorMessage),
        slog.Int("status_code", resp.StatusCode))

    return "", fmt.Errorf("Twitter API Error: %s", errorMessage)
}

func (s *PostTweetActionService) CanHandle(actionService string) bool {
    return actionService == "post_tweet"
}

type TwitterCredentials struct {
    ConsumerKey       string
    ConsumerSecret    string
    AccessToken       string
    AccessTokenSecret string
}

func extractTwitterCredentials(config map[string]interface{}) (*TwitterCredentials, error) {
    credentials := &TwitterCredentials{}
    var ok bool

    if credentials.ConsumerKey, ok = config["consumer_key"].(string); !ok {
        return nil, fmt.Errorf("consumer_key not found in config")
    }
    if credentials.ConsumerSecret, ok = config["consumer_secret"].(string); !ok {
        return nil, fmt.Errorf("consumer_secret not found in config")
    }
    if credentials.AccessToken, ok = config["access_token"].(string); !ok {
        return nil, fmt.Errorf("access_token not found in config")
    }
    if credentials.AccessTokenSecret, ok = config["access_token_secret"].(string); !ok {
        return nil, fmt.Errorf("access_token_secret not found in config")
    }

    return credentials, nil
}

// Helper functions
func cleanJsonContent(content string) string {
    content = trimPrefix(content, "```json")
    content = trimSuffix(content, "```")
    return content
}

func trimPrefix(s, prefix string) string {
    if len(s) > len(prefix) && s[:len(prefix)] == prefix {
        return s[len(prefix):]
    }
    return s
}

func trimSuffix(s, suffix string) string {
    if len(s) > len(suffix) && s[len(s)-len(suffix):] == suffix {
        return s[:len(s)-len(suffix)]
    }
    return s
}