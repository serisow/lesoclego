package rag_service

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"

    "github.com/pgvector/pgvector-go"
)

type EmbeddingRequest struct {
    Input string `json:"input"`
    Model string `json:"model"`
}

type EmbeddingResponse struct {
    Data []struct {
        Embedding *pgvector.Vector `json:"embedding"`
        TokenCount int            `json:"token_count"`
    } `json:"data"`
    Usage struct {
        TotalTokens int `json:"total_tokens"`
    } `json:"usage"`
    Object string `json:"object"`
}

func GetEmbeddingWithTokenCount(text string) (*pgvector.Vector, int, error) {
    apiKey := os.Getenv("OPENAI_API_KEY")
    if apiKey == "" {
        return nil, 0, fmt.Errorf("OPENAI_API_KEY not set")
    }

    requestBody := EmbeddingRequest{
        Input: text,
        Model: "text-embedding-ada-002",
    }

    jsonData, err := json.Marshal(requestBody)
    if err != nil {
        return nil, 0, fmt.Errorf("failed to marshal embedding request: %v", err)
    }

    req, err := http.NewRequest("POST", "https://api.openai.com/v1/embeddings", bytes.NewBuffer(jsonData))
    if err != nil {
        return nil, 0, fmt.Errorf("failed to create HTTP request: %v", err)
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+apiKey)

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return nil, 0, fmt.Errorf("failed to send HTTP request: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, 0, fmt.Errorf("embedding service returned status %d: %s", resp.StatusCode, string(body))
    }

    var embeddingResp EmbeddingResponse
    decoder := json.NewDecoder(resp.Body)
    if err := decoder.Decode(&embeddingResp); err != nil {
        return nil, 0, fmt.Errorf("failed to decode embedding response: %v", err)
    }

    if len(embeddingResp.Data) == 0 {
        return nil, 0, fmt.Errorf("no embedding data received")
    }

    return embeddingResp.Data[0].Embedding, embeddingResp.Usage.TotalTokens, nil
}