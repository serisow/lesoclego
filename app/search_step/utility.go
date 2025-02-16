package search_step

import (
    "regexp"
    "strings"
)

// cleanSearchContent is a shared utility function for cleaning content from various search sources.
// It handles common cleanup tasks needed by both Google Search and News API implementations.
func cleanSearchContent(content string, options ...cleanOption) string {
    // Default configuration
    config := &cleanConfig{
        maxLength: 2000,
        truncateSuffix: "...",
        removeCommonPrefixes: true,
        normalizeWhitespace: true,
    }

    // Apply any provided options
    for _, opt := range options {
        opt(config)
    }

    if config.normalizeWhitespace {
        // Remove extra whitespace
        content = regexp.MustCompile(`\s+`).ReplaceAllString(content, " ")
    }

    if config.removeCommonPrefixes {
        // Remove common article prefixes and metadata
        prefixPattern := regexp.MustCompile(`^(Share|Comments|Published|By|Author|Posted|Updated).+?\n`)
        content = prefixPattern.ReplaceAllString(content, "")
    }

    content = strings.TrimSpace(content)

    // Truncate if needed
    if config.maxLength > 0 && len(content) > config.maxLength {
        content = content[:config.maxLength]
        // Try to end at a proper sentence or word boundary
        if idx := strings.LastIndex(content, ". "); idx > config.maxLength/2 {
            content = content[:idx+1]
        } else if idx := strings.LastIndex(content, " "); idx > config.maxLength/2 {
            content = content[:idx]
        }
        content += config.truncateSuffix
    }

    return content
}

// cleanConfig holds configuration options for content cleaning
type cleanConfig struct {
    maxLength           int
    truncateSuffix      string
    removeCommonPrefixes bool
    normalizeWhitespace  bool
}

// cleanOption is a function type for configuring content cleaning
type cleanOption func(*cleanConfig)

// WithMaxLength sets the maximum length for content
func WithMaxLength(length int) cleanOption {
    return func(c *cleanConfig) {
        c.maxLength = length
    }
}

// WithTruncateSuffix sets the suffix to use when truncating content
func WithTruncateSuffix(suffix string) cleanOption {
    return func(c *cleanConfig) {
        c.truncateSuffix = suffix
    }
}

// WithoutPrefixRemoval disables removal of common prefixes
func WithoutPrefixRemoval() cleanOption {
    return func(c *cleanConfig) {
        c.removeCommonPrefixes = false
    }
}

// WithoutWhitespaceNormalization disables whitespace normalization
func WithoutWhitespaceNormalization() cleanOption {
    return func(c *cleanConfig) {
        c.normalizeWhitespace = false
    }
}