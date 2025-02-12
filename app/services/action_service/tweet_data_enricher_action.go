package action_service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/serisow/lesocle/pipeline_type"
)

const (
	TweetDataEnricherServiceName = "tweet_data_enricher"
)

type EnricherConfig struct {
	IncludeTweetURLs    bool `json:"include_tweet_urls"`
	IncludeUserProfiles bool `json:"include_user_profiles"`
}

type TweetDataEnricherService struct {
	logger *slog.Logger
}

func NewTweetDataEnricherService(logger *slog.Logger) *TweetDataEnricherService {
	return &TweetDataEnricherService{
		logger: logger,
	}
}

func (s *TweetDataEnricherService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
	if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
		return "", fmt.Errorf("missing action configuration for TweetDataEnricher")
	}

	config, err := extractEnricherConfig(step.ActionDetails.Configuration)
	if err != nil {
		return "", fmt.Errorf("error extracting configuration: %w", err)
	}

	// Get data from previous steps
	var tweetSearchData, crisisAnalysis map[string]interface{}

	// Get tweet search content
	tweetContent, ok := pipelineContext.StepOutputs["tweeter_search_content"].(string)
	if !ok {
		return "", fmt.Errorf("tweet search content not found in context")
	}
	if err := json.Unmarshal([]byte(tweetContent), &tweetSearchData); err != nil {
		return "", fmt.Errorf("error parsing tweet search data: %w", err)
	}

	// Get analysis result
	analysisContent, ok := pipelineContext.StepOutputs["analysis_result"].(string)
	if !ok {
		return "", fmt.Errorf("analysis result not found in context")
	}
	if err := json.Unmarshal([]byte(analysisContent), &crisisAnalysis); err != nil {
		return "", fmt.Errorf("error parsing crisis analysis data: %w", err)
	}

	// Create high priority tweets lookup
	highPriorityTweets := make(map[string]string)
	if priorityTweets, ok := crisisAnalysis["high_priority_tweets"].([]interface{}); ok {
		for _, pt := range priorityTweets {
			if ptMap, ok := pt.(map[string]interface{}); ok {
				if tweetID, ok := ptMap["tweet_id"].(string); ok {
					if reason, ok := ptMap["reason"].(string); ok {
						highPriorityTweets[tweetID] = reason
					}
				}
			}
		}
	}

	// Process tweets
	var enrichedTweets []map[string]interface{}
	if data, ok := tweetSearchData["data"].(map[string]interface{}); ok {
		if tweets, ok := data["tweets"].([]interface{}); ok {
			for _, t := range tweets {
				tweet, ok := t.(map[string]interface{})
				if !ok {
					continue
				}

				tweetID := fmt.Sprintf("%v", tweet["id"])
				tweetURL := ""
				userProfile := ""

				if config.IncludeTweetURLs {
					tweetURL = fmt.Sprintf("https://twitter.com/i/web/status/%s", tweetID)
				}
				if config.IncludeUserProfiles {
					if authorID, ok := tweet["author_id"].(string); ok {
						userProfile = fmt.Sprintf("https://twitter.com/i/user/%s", authorID)
					}
				}

				enrichedTweet := map[string]interface{}{
					"id":         tweetID,
					"text":       tweet["text"],
					"created_at": tweet["created_at"],
					"author": map[string]interface{}{
						"id":          tweet["author_id"],
						"profile_url": userProfile,
					},
					"metrics":          tweet["metrics"],
					"tweet_url":        tweetURL,
					"is_high_priority": false,
					"priority_reason":  nil,
				}

				if reason, exists := highPriorityTweets[tweetID]; exists {
					enrichedTweet["is_high_priority"] = true
					enrichedTweet["priority_reason"] = reason
				}

				if entities, ok := tweet["entities"].(map[string]interface{}); ok {
					enrichedTweet["entities"] = entities
				}

				enrichedTweets = append(enrichedTweets, enrichedTweet)
			}
		}
	}

	// Filter high priority tweets
	var highPriorityEnrichedTweets []map[string]interface{}
	for _, tweet := range enrichedTweets {
		if isHighPriority, ok := tweet["is_high_priority"].(bool); ok && isHighPriority {
			highPriorityEnrichedTweets = append(highPriorityEnrichedTweets, tweet)
		}
	}

	// Build final enriched data structure
	enrichedData := map[string]interface{}{
		"crisis_metrics": map[string]interface{}{
			"severity_score":     crisisAnalysis["severity_score"],
			"sentiment_analysis": crisisAnalysis["sentiment_analysis"],
			"viral_potential":    crisisAnalysis["viral_potential"],
		},
		"tweets": map[string]interface{}{
			"all_tweets":          enrichedTweets,
			"high_priority_tweets": highPriorityEnrichedTweets,
		},
		"metadata": map[string]interface{}{
			"total_tweets":        len(enrichedTweets),
			"high_priority_count": len(highPriorityEnrichedTweets),
			"search_query":        tweetSearchData["data"].(map[string]interface{})["metadata"].(map[string]interface{})["query"],
			"timestamp":          time.Now().Unix(),
		},
		"recommended_actions": crisisAnalysis["recommended_actions"],
	}

	resultJSON, err := json.Marshal(enrichedData)
	if err != nil {
		return "", fmt.Errorf("error marshaling enriched data: %w", err)
	}

	return string(resultJSON), nil
}

func (s *TweetDataEnricherService) CanHandle(actionService string) bool {
	return actionService == TweetDataEnricherServiceName
}

func extractEnricherConfig(config map[string]interface{}) (*EnricherConfig, error) {
	ec := &EnricherConfig{
		IncludeTweetURLs:    true, // Default to true
		IncludeUserProfiles: true, // Default to true
	}

	if includeTweetURLs, ok := config["include_tweet_urls"].(float64); ok {
		ec.IncludeTweetURLs = includeTweetURLs == 1
	} else if includeTweetURLs, ok := config["include_tweet_urls"].(bool); ok {
		ec.IncludeTweetURLs = includeTweetURLs
	}

	if includeUserProfiles, ok := config["include_user_profiles"].(float64); ok {
		ec.IncludeUserProfiles = includeUserProfiles == 1
	} else if includeUserProfiles, ok := config["include_user_profiles"].(bool); ok {
		ec.IncludeUserProfiles = includeUserProfiles
	}

	return ec, nil
}