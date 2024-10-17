package search_step_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serisow/lesocle/pipeline_type"
	"github.com/serisow/lesocle/search_step"
)

// search_step/google_search_step_test.go

func TestGoogleSearchStepImpl_Execute(t *testing.T) {
    tests := []struct {
        name                 string
        config               *pipeline_type.GoogleSearchConfig
        expectedOutputExists bool
        expectedError        bool
    }{
        {
            name: "Successful execution",
            config: &pipeline_type.GoogleSearchConfig{
                Query: "golang testing",
                AdvancedParams: pipeline_type.GoogleSearchParams{
                    NumResults: "1",
                },
            },
            expectedOutputExists: true,
            expectedError:        false,
        },
        {
            name: "Missing API Key",
            config: &pipeline_type.GoogleSearchConfig{
                Query: "golang testing",
            },
            expectedOutputExists: false,
            expectedError:        true,
        },
        // Additional test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup mock content server
            mockContentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                fmt.Fprint(w, `<html><body><article>This is the content of the article.</article></body></html>`)
            }))
            defer mockContentServer.Close()

            // Setup mock Google API server
            mockGoogleAPIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Content-Type", "application/json")
                fmt.Fprintf(w, `{
                    "items": [
                        {
                            "title": "Test Title",
                            "link": "%s",
                            "snippet": "Test snippet."
                        }
                    ]
                }`, mockContentServer.URL)
            }))
            defer mockGoogleAPIServer.Close()

            // Use the mock servers' client
            httpClient := mockGoogleAPIServer.Client()

            // Set API key and search engine ID based on the test case
            apiKey := "dummy-api-key"
            searchEngineID := "dummy-cx"
            if tt.name == "Missing API Key" {
                apiKey = ""
                searchEngineID = ""
            }

            step := &search_step.GoogleSearchStepImpl{
                PipelineStep: pipeline_type.PipelineStep{
                    StepOutputKey:      "search_results",
                    GoogleSearchConfig: tt.config,
                },
                HttpClient:       httpClient,
                GoogleAPIBaseURL: mockGoogleAPIServer.URL,
                APIKey:           apiKey,
                SearchEngineID:   searchEngineID,
            }

            ctx := pipeline_type.NewContext()

            err := step.Execute(context.Background(), ctx)
            if tt.expectedError {
                if err == nil {
                    t.Errorf("Expected an error but got none")
                }
            } else {
                if err != nil {
                    t.Errorf("Did not expect an error but got: %v", err)
                }
                output, exists := ctx.GetStepOutput("search_results")
                if !exists {
                    t.Errorf("Expected output not found in context")
                } else {
                    // Optionally, verify the content of the output
                    var results []map[string]string
                    if err := json.Unmarshal([]byte(output.(string)), &results); err != nil {
                        t.Errorf("Error unmarshalling output: %v", err)
                    }
                    if len(results) != 1 {
                        t.Errorf("Expected 1 result, got %d", len(results))
                    }
                    if results[0]["expanded_content"] != "This is the content of the article." {
                        t.Errorf("Unexpected expanded content: %s", results[0]["expanded_content"])
                    }
                }
            }
        })
    }
}
