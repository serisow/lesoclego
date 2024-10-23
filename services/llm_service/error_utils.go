package llm_service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OpenAIError represents the error structure returned by OpenAI API
type OpenAIError struct {
    Error struct {
        Message string `json:"message"`
        Type    string `json:"type"`
        Code    string `json:"code"`
    } `json:"error"`
}

type OpenAIHttpError struct {
    StatusCode int
    Message    string
    ErrorType  string
    RawBody    string
}

func (e *OpenAIHttpError) Error() string {
    return fmt.Sprintf("OpenAI API error (HTTP %d): %s (Type: %s)", e.StatusCode, e.Message, e.ErrorType)
}

// extractOpenAIErrorDetails extracts error information from OpenAI API responses
func extractOpenAIErrorDetails(resp *http.Response) (string, *OpenAIError) {
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", nil
    }

    // Try to parse as OpenAI error format
    var openAIErr OpenAIError
    if err := json.Unmarshal(body, &openAIErr); err == nil && openAIErr.Error.Message != "" {
        return string(body), &openAIErr
    }

    return string(body), nil
}