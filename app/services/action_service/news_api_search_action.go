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

	"github.com/PuerkitoBio/goquery"
	"github.com/serisow/lesocle/pipeline_type"
)

const (
	NewsSearchServiceName = "news_api_search"
	newsAPIBaseURL       = "https://newsapi.org/v2/everything"
)

type NewsSearchActionService struct {
	logger     *slog.Logger
	httpClient *http.Client
}

type NewsAPIConfig struct {
	APIKey    string `json:"api_key"`
	Query     string `json:"query"`
	Language  string `json:"language"`
	SortBy    string `json:"sort_by"`
	PageSize  int    `json:"page_size"`
}

func NewNewsSearchActionService(logger *slog.Logger) *NewsSearchActionService {
	return &NewsSearchActionService{
		logger: logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *NewsSearchActionService) Execute(ctx context.Context, actionConfig string, pipelineContext *pipeline_type.Context, step *pipeline_type.PipelineStep) (string, error) {
	if step.ActionDetails == nil || step.ActionDetails.Configuration == nil {
		return "", fmt.Errorf("missing action configuration for NewsSearchAction")
	}

	config, err := extractNewsAPIConfig(step.ActionDetails.Configuration)
	if err != nil {
		return "", fmt.Errorf("error extracting News API configuration: %w", err)
	}

	// Process dynamic query from required steps
	query := config.Query
	if step.RequiredSteps != "" {
		requiredSteps := strings.Split(step.RequiredSteps, "\r\n")
		for _, requiredStep := range requiredSteps {
			requiredStep = strings.TrimSpace(requiredStep)
			if requiredStep == "" {
				continue
			}
			if stepOutput, ok := pipelineContext.GetStepOutput(requiredStep); ok {
				placeholder := fmt.Sprintf("{%s}", requiredStep)
				query = strings.Replace(query, placeholder, fmt.Sprintf("%v", stepOutput), -1)
			}
		}
	}

	// Build request parameters
	params := url.Values{}
	params.Set("q", query)
	params.Set("language", config.Language)
	params.Set("sortBy", config.SortBy)
	params.Set("pageSize", fmt.Sprintf("%d", config.PageSize))
	params.Set("apiKey", config.APIKey)

	// Make the request
	reqURL := fmt.Sprintf("%s?%s", newsAPIBaseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error("Failed to fetch news",
			slog.String("error", err.Error()),
			slog.String("query", query))
		return "", fmt.Errorf("error fetching news: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
			return "", fmt.Errorf("news API error (status %d)", resp.StatusCode)
		}
		return "", fmt.Errorf("news API error: %s", errorResp.Message)
	}

	var apiResponse struct {
		TotalResults int `json:"totalResults"`
		Articles     []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			URL         string `json:"url"`
			PublishedAt string `json:"publishedAt"`
			Source      struct {
				Name string `json:"name"`
			} `json:"source"`
			Author    string `json:"author"`
			URLToImage string `json:"urlToImage"`
		} `json:"articles"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	// Format results with expanded content
	formattedResults := map[string]interface{}{
		"query":         query,
		"total_results": apiResponse.TotalResults,
		"articles":      []map[string]interface{}{},
		"metadata": map[string]interface{}{
			"timestamp": time.Now().Unix(),
			"language":  config.Language,
			"sort_by":   config.SortBy,
		},
	}

	// Process articles with expanded content
	for _, article := range apiResponse.Articles {
		expandedContent := s.fetchExpandedContent(article.URL)

		formattedArticle := map[string]interface{}{
			"title":           article.Title,
			"description":     article.Description,
			"url":            article.URL,
			"published_at":    article.PublishedAt,
			"source":         article.Source.Name,
			"author":         article.Author,
			"image_url":      article.URLToImage,
			"expanded_content": expandedContent,
		}

		formattedResults["articles"] = append(formattedResults["articles"].([]map[string]interface{}), formattedArticle)
	}

	resultJSON, err := json.Marshal(formattedResults)
	if err != nil {
		return "", fmt.Errorf("error marshaling results: %w", err)
	}

	return string(resultJSON), nil
}

func (s *NewsSearchActionService) CanHandle(actionService string) bool {
	return actionService == NewsSearchServiceName
}

func extractNewsAPIConfig(config map[string]interface{}) (*NewsAPIConfig, error) {
	apiConfig := &NewsAPIConfig{
		Language: "en",    // Default value
		SortBy:   "publishedAt",
		PageSize: 20,     // Default value
	}

	var ok bool
	if apiConfig.APIKey, ok = config["api_key"].(string); !ok {
		return nil, fmt.Errorf("api_key not found in config")
	}
	if apiConfig.Query, ok = config["query"].(string); !ok {
		return nil, fmt.Errorf("query not found in config")
	}

	// Optional parameters with defaults
	if language, ok := config["language"].(string); ok {
		apiConfig.Language = language
	}
	if sortBy, ok := config["sort_by"].(string); ok {
		apiConfig.SortBy = sortBy
	}
	if pageSize, ok := config["page_size"].(float64); ok {
		apiConfig.PageSize = int(pageSize)
	}

	return apiConfig, nil
}

func (s *NewsSearchActionService) fetchExpandedContent(articleURL string) string {
	if strings.Contains(articleURL, "consent.yahoo.com") {
		return "Content unavailable - requires consent"
	}

	req, err := http.NewRequest("GET", articleURL, nil)
	if err != nil {
		return fmt.Sprintf("Error creating request: %s", err.Error())
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GoNewsAPI/1.0)")
	
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Sprintf("Error fetching content: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("Error fetching content: HTTP status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return fmt.Sprintf("Error parsing HTML: %s", err.Error())
	}

	// Try article-specific selectors first
	contentSelectors := []string{
		"article.article-content",
		"div.article-body",
		"div.story-content",
		"div.post-content",
		"main article",
		"div[role='main']",
	}

	var content string
	for _, selector := range contentSelectors {
		doc.Find(selector).Each(func(i int, s *goquery.Selection) {
			content += s.Text() + "\n"
		})
		if len(content) > 100 {
			break
		}
	}

	// Fallback to basic content extraction
	if content == "" {
		doc.Find("article, div.content").Each(func(i int, s *goquery.Selection) {
			content += s.Text() + "\n"
		})
	}

	if content == "" {
		return "No article content found."
	}

	// Clean content
	content = s.cleanContent(content)
	
	// Truncate to reasonable length
	if len(content) > 800 {
		lastPeriod := strings.LastIndex(content[:800], ".")
		if lastPeriod > 0 {
			content = content[:lastPeriod+1]
		}
	}

	return content
}

func (s *NewsSearchActionService) cleanContent(content string) string {
	// Remove extra whitespace
	content = strings.Join(strings.Fields(content), " ")

	// Remove common cruft
	cleanPatterns := []string{
		`^Share.*\n`,
		`^Comments.*\n`,
		`^Published.*\n`,
		`^By.*\n`,
		`^Author.*\n`,
	}

	for _, pattern := range cleanPatterns {
		content = strings.ReplaceAll(content, pattern, "")
	}

	return strings.TrimSpace(content)
}