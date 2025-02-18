package search_step

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/serisow/lesocle/config"
	"github.com/serisow/lesocle/pipeline_type"
)

type NewsAPISearchStepImpl struct {
    PipelineStep     pipeline_type.PipelineStep
    HttpClient       *http.Client
    NewsAPIBaseURL   string
    APIKey           string
}

func (s *NewsAPISearchStepImpl) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    if s.PipelineStep.NewsAPIConfig == nil {
        return fmt.Errorf("news API configuration is missing")
    }

    apiKey := s.APIKey

    // Fallback to config if value is empty
    if apiKey == "" {
        cfg := config.Load()
        apiKey = cfg.NewsAPIKey
    }

    if apiKey == "" {
        return fmt.Errorf("news API key is not configured")
    }

    // Use injected base URL or default
    baseURL := s.NewsAPIBaseURL
    if baseURL == "" {
        baseURL = "https://newsapi.org/v2/everything"
    }

    // Use injected HTTP client or default
    client := s.HttpClient
    if client == nil {
        client = &http.Client{
            Timeout: 30 * time.Second,
        }
    }

    // Process query with context variables
    query := s.PipelineStep.NewsAPIConfig.Query
    if s.PipelineStep.RequiredSteps != "" {
        for _, stepKey := range strings.Split(s.PipelineStep.RequiredSteps, "\r\n") {
            stepKey = strings.TrimSpace(stepKey)
            if stepKey == "" {
                continue
            }
            if value, exists := pipelineContext.GetStepOutput(stepKey); exists {
                placeholder := fmt.Sprintf("{%s}", stepKey)
                query = strings.Replace(query, placeholder, fmt.Sprint(value), -1)
            }
        }
    }

    // Build query parameters
    params := url.Values{}
    params.Set("q", query)
    params.Set("language", s.PipelineStep.NewsAPIConfig.AdvancedParams.Language)
    params.Set("sortBy", s.PipelineStep.NewsAPIConfig.AdvancedParams.SortBy)
    params.Set("pageSize", s.PipelineStep.NewsAPIConfig.AdvancedParams.PageSize)

    // Add date range if specified
    if s.PipelineStep.NewsAPIConfig.AdvancedParams.DateRange.From != "" {
        params.Set("from", s.PipelineStep.NewsAPIConfig.AdvancedParams.DateRange.From)
    }
    if s.PipelineStep.NewsAPIConfig.AdvancedParams.DateRange.To != "" {
        params.Set("to", s.PipelineStep.NewsAPIConfig.AdvancedParams.DateRange.To)
    }

    // Create request
    req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"?"+params.Encode(), nil)
    if err != nil {
        return fmt.Errorf("error creating request: %w", err)
    }

    req.Header.Set("X-Api-Key", apiKey)
    req.Header.Set("User-Agent", "Pipeline/1.0")

    // Execute request
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("error making News API request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("news API returned non-200 status code: %d, body: %s", resp.StatusCode, string(body))
    }

    // Parse response
    var result struct {
        Status       string `json:"status"`
        TotalResults int    `json:"totalResults"`
        Articles     []struct {
            Title       string `json:"title"`
            Description string `json:"description"`
            URL         string `json:"url"`
            PublishedAt string `json:"publishedAt"`
            Source      struct {
                Name string `json:"name"`
            } `json:"source"`
            Author     string `json:"author"`
            URLToImage string `json:"urlToImage"`
        } `json:"articles"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return fmt.Errorf("error decoding News API response: %w", err)
    }

    // Format results
    formattedResults := map[string]interface{}{
        "query":         query,
        "total_results": result.TotalResults,
        "articles":      make([]map[string]interface{}, 0),
        "metadata": map[string]interface{}{
            "timestamp": time.Now().Unix(),
            "language":  s.PipelineStep.NewsAPIConfig.AdvancedParams.Language,
            "sort_by":   s.PipelineStep.NewsAPIConfig.AdvancedParams.SortBy,
        },
    }

    for _, article := range result.Articles {
        expandedContent, err := s.fetchExpandedContent(article.URL)
		if err != nil {
			// @TODO log the error or take better approach
			continue
		}

        articleData := map[string]interface{}{
            "title":            article.Title,
            "description":      article.Description,
            "url":             article.URL,
            "published_at":     article.PublishedAt,
            "source":          article.Source.Name,
            "author":          article.Author,
            "image_url":       article.URLToImage,
            "expanded_content": expandedContent,
        }
        formattedResults["articles"] = append(formattedResults["articles"].([]map[string]interface{}), articleData)
    }

    // Store results in context
    resultJSON, err := json.Marshal(formattedResults)
    if err != nil {
        return fmt.Errorf("error marshaling results: %w", err)
    }

    pipelineContext.SetStepOutput(s.PipelineStep.StepOutputKey, string(resultJSON))
    return nil
}

func (s *NewsAPISearchStepImpl) fetchExpandedContent(url string) (string, error) {
	if strings.Contains(url, "consent.yahoo.com") {
		return "Content unavailable - requires consent", nil
	}

	// Configure HTTP client
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Drupal/10.0; +http://example.com)")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Parse HTML document
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	// First pass: Remove unwanted tags
	tagsToRemove := []string{
		"script", "style", "iframe", "noscript", "nav",
		"footer", "header", "form", "button", "input",
		"select", "textarea", "svg", "img", "meta",
		"link", "object", "embed", "applet", "frame",
		"frameset", "map", "area", "audio", "video",
		"source", "track", "canvas", "datalist", "keygen",
		"output", "progress", "time",
	}
	for _, tag := range tagsToRemove {
		doc.Find(tag).Each(func(i int, s *goquery.Selection) {
			s.Remove()
		})
	}

	// Second pass: Remove elements by class/id patterns
	unwantedSelectors := []string{
		"*[class*='ad']",
		"*[class*='banner']",
		"*[class*='sidebar']",
		"*[class*='popup']",
		"*[class*='cookie']",
		"*[class*='modal']",
		"*[class*='newsletter']",
		"*[id*='ad']",
		"*[id*='banner']",
		"*[id*='sidebar']",
		"*[id*='popup']",
		"*[id*='cookie']",
		"*[id*='modal']",
		"*[id*='newsletter']",
		"*[role='navigation']",
		"*[role='banner']",
		"*[role='complementary']",
		"*[role='search']",
	}
	for _, selector := range unwantedSelectors {
		doc.Find(selector).Each(func(i int, s *goquery.Selection) {
			s.Remove()
		})
	}

	// Third pass: Content heuristics
	const (
		minTextLength  = 120
		maxLinkDensity = 0.25
	)

	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		text := strings.Join(strings.Fields(s.Text()), " ")
		textLength := utf8.RuneCountInString(text)
		linkCount := s.Find("a").Length()
		linkDensity := 0.0
		if textLength > 0 {
			linkDensity = float64(linkCount) / float64(textLength)
		}

		if textLength < minTextLength || linkDensity > maxLinkDensity || text == "" {
			s.Remove()
		}
	})

	// Final cleanup
	text := doc.Find("body").Text()
	
	// Cleanup regex patterns
	cleanupPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\s+`),
		regexp.MustCompile(`\[\d+\]`),
		regexp.MustCompile(`https?:\/\/\S+`),
	}

	for _, pattern := range cleanupPatterns {
		text = pattern.ReplaceAllString(text, " ")
	}

	return strings.TrimSpace(text), nil
}

func (s *NewsAPISearchStepImpl) GetType() string {
    return "news_api_search"
}