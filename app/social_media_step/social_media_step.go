package social_media_step

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/serisow/lesocle/pipeline_type"
)

type SocialMediaStepImpl struct {
    PipelineStep pipeline_type.PipelineStep
}

type ArticleData struct {
    Nid     string `json:"nid"`
    Title   string `json:"title"`
    Body    string `json:"body"`
    Summary string `json:"summary"`
    URL     string `json:"url"`
    ImageURL string `json:"image_url,omitempty"`
}

func (s *SocialMediaStepImpl) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    // Parse article data from the pipeline step configuration
    var articleData ArticleData
    if s.PipelineStep.ArticleData == nil {
        return fmt.Errorf("article data is missing in step configuration")
    }

    jsonData, err := json.Marshal(s.PipelineStep.ArticleData)
    if err != nil {
        return fmt.Errorf("error marshaling article data: %v", err)
    }

    if err := json.Unmarshal(jsonData, &articleData); err != nil {
        return fmt.Errorf("error parsing article data: %v", err)
    }
    // Create metadata structure
    metadata := map[string]interface{}{
        "article_nid": articleData.Nid,
        "title":      articleData.Title,
        "url":        articleData.URL,
        "image_url":  articleData.ImageURL,
        "summary":    articleData.Summary,
        "body":       articleData.Body,
    }

    // Generate Twitter content
    tweetText := s.generateTweet(metadata)
    twitterContent := map[string]string{
        "text": tweetText,
    }

    // Generate LinkedIn content
    linkedinText := s.generateLinkedInPost(metadata)
    linkedinContent := map[string]interface{}{
        "text": linkedinText,
        "media": map[string]interface{}{
            "url":         articleData.URL,
            "title":      articleData.Title,
            "description": s.generateSummary(articleData.Summary, 200),
            "originalUrl": articleData.URL,  // This is required for LinkedIn's API
            "status": "READY",              // Required status
        },
    }

    if articleData.ImageURL != "" {
        linkedinContent["media"].(map[string]interface{})["thumbnail"] = articleData.ImageURL
    }
    // Create the final result structure
    result := map[string]interface{}{
        "article_id": articleData.Nid,
        "platforms": map[string]interface{}{
            "twitter":  twitterContent,
            "linkedin": linkedinContent,
        },
        "metadata": metadata,
    }

    // Convert to JSON and store in the step output
    resultJSON, err := json.Marshal(result)
    if err != nil {
        return fmt.Errorf("error marshaling result: %v", err)
    }

    pipelineContext.SetStepOutput(s.PipelineStep.StepOutputKey, string(resultJSON))
    return nil
}

func (s *SocialMediaStepImpl) generateTweet(metadata map[string]interface{}) string {
    title := metadata["title"].(string)
    summary := s.generateSummary(metadata["summary"].(string), 180) // Shorter for tweet
    url := metadata["url"].(string)

    tweet := fmt.Sprintf("%s\n\n%s", title, summary)
    return s.ensureTweetLength(tweet, url)
}

func (s *SocialMediaStepImpl) generateLinkedInPost(metadata map[string]interface{}) string {
    title := metadata["title"].(string)
    summary := s.generateSummary(metadata["summary"].(string), 500) // Longer for LinkedIn

    parts := []string{
        title,
        "",  // Empty line after title
        summary,
        "",  // Empty line before call to action
        "🔍 Read the full article for more details.",
    }

    return strings.Join(parts, "\n")
}

func (s *SocialMediaStepImpl) generateSummary(text string, length int) string {
    if len(text) <= length {
        return text
    }

    // Try to cut at a sentence boundary
    if idx := strings.LastIndex(text[:length], ". "); idx > length/2 {
        return text[:idx+1]
    }

    // Fall back to word boundary
    if idx := strings.LastIndex(text[:length], " "); idx > 0 {
        return text[:idx] + "..."
    }

    return text[:length] + "..."
}

func (s *SocialMediaStepImpl) ensureTweetLength(tweet string, url string) string {
    const (
        urlLength  = 23 // Twitter treats all URLs as 23 characters
        maxLength  = 280
        separator  = " "
    )

    availableLength := maxLength - urlLength - len(separator)
    if len(tweet) > availableLength {
        if idx := strings.LastIndex(tweet[:availableLength], " "); idx > 0 {
            tweet = tweet[:idx] + "…"
        } else {
            tweet = tweet[:availableLength-1] + "…"
        }
    }

    return tweet + separator + url
}

func (s *SocialMediaStepImpl) GetType() string {
    return "social_media_step"
}