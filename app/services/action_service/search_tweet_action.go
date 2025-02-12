package action_service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dghubble/oauth1"
	"github.com/serisow/lesocle/pipeline_type"
)

const (
	SearchTweetsServiceName = "search_tweets"
	twitterAPIV2BaseURL     = "https://api.twitter.com/2"
)

type TwitterSearchConfig struct {
	ConsumerKey       string `json:"consumer_key"`
	ConsumerSecret    string `json:"consumer_secret"`
	AccessToken       string `json:"access_token"`
	AccessTokenSecret string `json:"access_token_secret"`
	SearchQuery       string `json:"search_query"`
	MaxResults        int    `json:"max_results"`
	IncludeMetrics    bool   `json:"include_metrics"`
	ResultType        string `json:"result_type"`
	IncludeEntities   bool   `json:"include_entities"`
}

type SearchTweetsActionService struct {
	logger *slog.Logger
}

func NewSearchTweetsActionService(logger *slog.Logger) *SearchTweetsActionService {
	return &SearchTweetsActionService{
		logger: logger,
	}
}

func (s *SearchTweetsActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
	if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
		return "", fmt.Errorf("missing action configuration for SearchTweetsAction")
	}

	config, err := extractTwitterSearchConfig(step.ActionDetails.Configuration)

	if err != nil {
		return "", fmt.Errorf("error extracting Twitter search configuration: %w", err)
	}

	// Process search query with context
	searchQuery := config.SearchQuery
	if step.RequiredSteps != "" {
		requiredSteps := strings.Split(step.RequiredSteps, "\r\n")
		for _, requiredStep := range requiredSteps {
			requiredStep = strings.TrimSpace(requiredStep)
			if requiredStep == "" {
				continue
			}
			if stepOutput, ok := pipelineContext.GetStepOutput(requiredStep); ok {
				placeholder := fmt.Sprintf("{%s}", requiredStep)
				searchQuery = strings.Replace(searchQuery, placeholder, fmt.Sprintf("%v", stepOutput), -1)
			}
		}
	}

	// Configure OAuth1.0a client
	oauthConfig := oauth1.NewConfig(config.ConsumerKey, config.ConsumerSecret)
	token := oauth1.NewToken(config.AccessToken, config.AccessTokenSecret)
	httpClient := oauthConfig.Client(ctx, token)

	// Build search URL with parameters
	params := url.Values{}
	params.Add("query", searchQuery)
	params.Add("max_results", fmt.Sprintf("%d", config.MaxResults))
	params.Add("tweet.fields", buildTweetFields(config))

	searchURL := fmt.Sprintf("%s/tweets/search/recent?%s", twitterAPIV2BaseURL, params.Encode())

	// Execute request
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	response, err := httpClient.Do(req)
	if err != nil {
		s.logger.Error("Error executing Twitter search request",
			slog.String("error", err.Error()))
		return "", fmt.Errorf("error executing request: %w", err)
	}
	defer response.Body.Close()

	// Handle rate limiting
	if response.StatusCode == 429 {
		s.logger.Warn("Rate limit exceeded for Twitter search",
			slog.String("reset", response.Header.Get("x-rate-limit-reset")))
		return "", fmt.Errorf("rate limit exceeded - please wait before trying again")
	}

	responseBody, err := s.processResponse(response)
	if err != nil {
		return "", err
	}

	// Parse and format the response
	var searchResult struct {
		Data []struct {
			ID            string `json:"id"`
			Text          string `json:"text"`
			CreatedAt     string `json:"created_at"`
			AuthorID      string `json:"author_id"`
			PublicMetrics struct {
				RetweetCount int `json:"retweet_count"`
				ReplyCount   int `json:"reply_count"`
				LikeCount    int `json:"like_count"`
				QuoteCount   int `json:"quote_count"`
			} `json:"public_metrics"`
			Entities map[string]interface{} `json:"entities"`
		} `json:"data"`
	}

	if err := json.Unmarshal(responseBody, &searchResult); err != nil {
		return "", fmt.Errorf("error parsing Twitter response: %w", err)
	}

	// Format results
	tweets := make([]map[string]interface{}, 0, len(searchResult.Data))
	for _, tweet := range searchResult.Data {
		tweetData := map[string]interface{}{
			"id":         tweet.ID,
			"text":       tweet.Text,
			"created_at": tweet.CreatedAt,
			"author_id":  tweet.AuthorID,
		}

		if config.IncludeMetrics {
			tweetData["metrics"] = map[string]int{
				"retweets": tweet.PublicMetrics.RetweetCount,
				"replies":  tweet.PublicMetrics.ReplyCount,
				"likes":    tweet.PublicMetrics.LikeCount,
				"quotes":   tweet.PublicMetrics.QuoteCount,
			}
		}

		if config.IncludeEntities && tweet.Entities != nil {
			tweetData["entities"] = tweet.Entities
		}

		tweets = append(tweets, tweetData)
	}

	// Prepare final response
	result := map[string]interface{}{
		"status":  "success",
		"service": "twitter_search",
		"data": map[string]interface{}{
			"tweets": tweets,
			"metadata": map[string]interface{}{
				"query":       searchQuery,
				"max_results": config.MaxResults,
				"found_count": len(tweets),
				"timestamp":   time.Now().Unix(),
				"result_type": config.ResultType,
			},
		},
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("error marshaling result: %w", err)
	}

	return string(resultJSON), nil
}

func (s *SearchTweetsActionService) CanHandle(actionService string) bool {
	return actionService == SearchTweetsServiceName
}

func extractTwitterSearchConfig(config map[string]interface{}) (*TwitterSearchConfig, error) {
	tsc := &TwitterSearchConfig{}

	var ok bool
	if tsc.ConsumerKey, ok = config["consumer_key"].(string); !ok {
		return nil, fmt.Errorf("consumer_key not found in config")
	}
	if tsc.ConsumerSecret, ok = config["consumer_secret"].(string); !ok {
		return nil, fmt.Errorf("consumer_secret not found in config")
	}
	if tsc.AccessToken, ok = config["access_token"].(string); !ok {
		return nil, fmt.Errorf("access_token not found in config")
	}
	if tsc.AccessTokenSecret, ok = config["access_token_secret"].(string); !ok {
		return nil, fmt.Errorf("access_token_secret not found in config")
	}
	if tsc.SearchQuery, ok = config["search_query"].(string); !ok {
		return nil, fmt.Errorf("search_query not found in config")
	}

	// For MaxResults: Change from float64 check to handle string or number
	if maxResults, ok := config["max_results"].(string); ok {
		if val, err := strconv.Atoi(maxResults); err == nil {
			tsc.MaxResults = val
		}
	} else if maxResults, ok := config["max_results"].(float64); ok {
		tsc.MaxResults = int(maxResults)
	} else {
		tsc.MaxResults = 10 // Default value
	}

	// For boolean fields: Handle both bool and number
	if includeMetrics, ok := config["include_metrics"].(float64); ok {
		tsc.IncludeMetrics = includeMetrics == 1
	} else if includeMetrics, ok := config["include_metrics"].(bool); ok {
		tsc.IncludeMetrics = includeMetrics
	}

	if includeEntities, ok := config["include_entities"].(float64); ok {
		tsc.IncludeEntities = includeEntities == 1
	} else if includeEntities, ok := config["include_entities"].(bool); ok {
		tsc.IncludeEntities = includeEntities
	}

	return tsc, nil
}

func (s *SearchTweetsActionService) processResponse(response *http.Response) ([]byte, error) {
	if response.StatusCode != http.StatusOK {
		var errResp struct {
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.NewDecoder(response.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("twitter API error (HTTP %d)", response.StatusCode)
		}
		if len(errResp.Errors) > 0 {
			return nil, fmt.Errorf("twitter API error: %s", errResp.Errors[0].Message)
		}
		return nil, fmt.Errorf("twitter API error (HTTP %d)", response.StatusCode)
	}

	var rawResponse json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&rawResponse); err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	return rawResponse, nil
}

func buildTweetFields(config *TwitterSearchConfig) string {
	fields := []string{"created_at", "author_id"}
	if config.IncludeMetrics {
		fields = append(fields, "public_metrics")
	}
	if config.IncludeEntities {
		fields = append(fields, "entities")
	}
	return strings.Join(fields, ",")
}
