package search_step

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/serisow/lesocle/config"
	"github.com/serisow/lesocle/pipeline_type"
)

type GoogleSearchStepImpl struct {
    pipeline_type.PipelineStep
}

func (s *GoogleSearchStepImpl) Execute(ctx context.Context, pipelineContext *pipeline_type.Context) error {
    if s.GoogleSearchConfig == nil {
        return fmt.Errorf("google search configuration is missing")
    }

    cfg := config.Load()
    apiKey := cfg.GoogleCustomSearchAPIKey
    cx := cfg.GoogleCustomSearchEngineID

    if apiKey == "" || cx == "" {
        return fmt.Errorf("google Custom Search API key or Search Engine ID is not configured")
    }

    query := s.GoogleSearchConfig.Query
    if s.GoogleSearchConfig.Category != "" {
        query += " " + s.GoogleSearchConfig.Category
    }

    baseURL := "https://www.googleapis.com/customsearch/v1"
    params := url.Values{}
    params.Set("key", apiKey)
    params.Set("cx", cx)
    params.Set("q", query)

    // Add advanced parameters
    advParams := s.GoogleSearchConfig.AdvancedParams
    if advParams.NumResults != "" {
        params.Set("num", advParams.NumResults)
    }
    if advParams.DateRestrict != "" {
        params.Set("dateRestrict", advParams.DateRestrict)
    }
    if advParams.Sort != "" {
        params.Set("sort", advParams.Sort)
    }
    if advParams.Language != "" {
        params.Set("lr", advParams.Language)
    }
    if advParams.Country != "" {
        params.Set("cr", advParams.Country)
    }
    if advParams.SiteSearch != "" {
        params.Set("siteSearch", advParams.SiteSearch)
    }
    if advParams.FileType != "" {
        params.Set("fileType", advParams.FileType)
    }
    if advParams.SafeSearch != "" {
        params.Set("safe", advParams.SafeSearch)
    }

    fullURL := baseURL + "?" + params.Encode()

    resp, err := http.Get(fullURL)
    if err != nil {
        return fmt.Errorf("error making Google search request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("google search API returned non-200 status code: %d", resp.StatusCode)
    }

    var searchResult map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
        return fmt.Errorf("error decoding Google search response: %w", err)
    }

    // Process and format the search results
    formattedResults, err := s.formatSearchResults(searchResult)
    if err != nil {
        return fmt.Errorf("error formatting search results: %w", err)
    }

    // Store the formatted results in the pipeline context
    pipelineContext.SetStepOutput(s.StepOutputKey, formattedResults)

    return nil
}

func (s *GoogleSearchStepImpl) formatSearchResults(searchResult map[string]interface{}) (string, error) {
    items, ok := searchResult["items"].([]interface{})
    if !ok {
        return "", fmt.Errorf("no search results found")
    }

    var formattedResults []map[string]string
    for _, item := range items {
        itemMap, ok := item.(map[string]interface{})
        if !ok {
            continue
        }

        link := s.getStringValue(itemMap, "link")
        expandedContent := s.fetchExpandedContent(link)

        formattedItem := map[string]string{
            "title":            s.getStringValue(itemMap, "title"),
            "link":             link,
            "snippet":          s.getStringValue(itemMap, "snippet"),
            "expanded_content": expandedContent,
        }
        formattedResults = append(formattedResults, formattedItem)
    }

    jsonResult, err := json.Marshal(formattedResults)
    if err != nil {
        return "", fmt.Errorf("error marshaling formatted results: %w", err)
    }

    return string(jsonResult), nil
}

func (s *GoogleSearchStepImpl) getStringValue(m map[string]interface{}, key string) string {
    if value, ok := m[key].(string); ok {
        return value
    }
    return ""
}

func (s *GoogleSearchStepImpl) fetchExpandedContent(url string) string {
    resp, err := http.Get(url)
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

    // Extract text from main content areas
    var content string
	doc.Find("article, .content, #content, main, .post, #main, .entry-content, .post-content, .blog-post, #primary, #main-content, .text, .text-content, #body-content, .post-article").Each(func(i int, s *goquery.Selection) {
		content += s.Text() + "\n"
	})
	

    // If no specific content found, get all text from body
    if content == "" {
        content = doc.Find("body").Text()
    }

    // Clean and truncate the content
    content = cleanContent(content)
    if len(content) > 2000 {
        content = content[:2000] + "..."
    }

    return content
}

func cleanContent(content string) string {
    // Remove extra whitespace
    content = regexp.MustCompile(`\s+`).ReplaceAllString(content, " ")
    return strings.TrimSpace(content)
}


func (s *GoogleSearchStepImpl) GetType() string {
    return "google_search"
}