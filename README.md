# LeSocle: Go Pipeline Execution Service

LeSocle is a sophisticated pipeline execution service written in Go that works in tandem with a Drupal backend system. It provides a robust framework for executing configurable, multi-step processing pipelines that can include various operations such as LLM interactions, web searches, social media actions, and content generation.

## Core Architecture

The service implements a modular architecture with several key components:

### 1. Pipeline Configuration & Execution

The pipeline system mirrors Drupal's configuration entities:
- `Pipeline` represents a sequence of steps to be executed
- `PipelineStep` defines individual operations within a pipeline 
- `Context` provides a shared data store for steps to exchange information
- Multiple execution modes support both scheduled and on-demand processing

### 2. Plugin Registry System

The `PluginRegistry` provides a flexible extension mechanism that mirrors Drupal's plugin architecture:
- Registers and manages different step types (LLM, action, search)
- Manages service instances for LLM providers and action handlers
- Allows dynamic registration of new step implementations
- Creates step instances at runtime based on configuration

### 3. Scheduler

The scheduler component manages timed execution of pipelines:
- Polls for scheduled pipelines from the Drupal backend
- Supports one-time and recurring schedules (daily, weekly, monthly)
- Prevents concurrent execution of the same pipeline
- Tracks execution failures and implements retry policies

### 4. HTTP Server

The service exposes HTTP endpoints for:
- On-demand pipeline execution
- Execution status tracking
- Result retrieval
- Media file serving (images, videos)

## Step Types and Integrations

### LLM Steps
- OpenAI: Text generation and DALL-E image creation
- Anthropic: Claude models for text generation
- Gemini: Google's Gemini models for text and image generation
- ElevenLabs & AWS Polly: Text-to-speech conversion

### Search Steps
- Google Custom Search: Retrieves and processes web search results
- News API: Searches for news articles with content extraction

### Action Steps
- Social Media: Posts to Twitter/X, LinkedIn, Facebook
- Video Generation: Creates videos from images and audio
- Webhook Integration: Sends data to external services
- SMS Sending: Delivers messages via Twilio

### Upload Steps
- Image Upload: Processes and stores images
- Audio Upload: Handles audio content
- Content enrichment: Adds metadata and overlays to media

## Drupal Integration

The service maintains a tight integration with its Drupal counterpart:
- Fetches pipeline configurations via REST API
- Reports execution results back to Drupal
- Respects Drupal's plugin architecture
- Handles authentication via configured credentials
- Maintains compatible data structures for seamless interchange

### API Contract

1. **Configuration Retrieval**:
   - GET `/pipelines/{id}` - Retrieves full pipeline configuration
   - GET `/pipelines/scheduled` - Gets scheduled pipelines

2. **Execution Management**:
   - POST `/pipeline/{id}/execute` - Triggers on-demand execution
   - GET `/pipeline/{id}/execution/{execution_id}/status` - Checks execution status
   - GET `/pipeline/{id}/execution/{execution_id}/results` - Retrieves results

3. **Result Reporting**:
   - POST `/pipeline/{id}/execution-result` - Reports execution results to Drupal

4. **Media Serving**:
   - GET `/api/videos/{file_id}` - Serves generated video files
   - GET `/api/images/{file_id}` - Serves generated image files

## Concurrency & State Management

The service implements sophisticated concurrency controls:
- Thread-safe execution store for tracking pipeline status
- Mutex-protected maps for running pipelines
- Batch processing for resource-intensive operations
- Configurable concurrency limits for external API calls
- Background cleanup of expired execution results

## Error Handling & Recovery

Robust error handling throughout the system:
- Retry mechanisms for transient external service failures
- Comprehensive error logging with context
- Error propagation to Drupal for visibility
- Step failure isolation to prevent pipeline abandonment
- Execution failure tracking with automated throttling

## Storage Management

The service manages generated content with:
- Structured directory hierarchy for different media types
- Timestamp-based organization for easy retrieval
- Scheduled cleanup of old files
- Metadata tracking for generated content
- URL scheme for consistent access

## Logging System

A comprehensive logging system with:
- Structured logs (slog)
- Daily rotating log files
- Log level configuration
- Component-specific loggers
- Detailed error and transaction logging

This Go service works as the execution engine for the Drupal-defined pipeline configurations, providing scalable, concurrent, and reliable processing while maintaining complete compatibility with Drupal's configuration and plugin systems.




# Useful commands

docker compose down && cd app/ && GOOS=linux go build -o lesoclego && cd .. && docker compose up



EXAMPLE call one-time execution:

curl -X POST http://lesoclego-dev.sa/pipeline/test_first_on_demand/execute \
     -H "Content-Type: application/json" \
     -d '{"user_input": "Analyze the impact of artificial intelligence on job markets"}'


     {"execution_id":"aa90167b-4f2a-4915-8e30-f50e094ab11c","links":{"results":"/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c/results","self":"/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c","status":"/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c/status"},"pipeline_id":"test_first_on_demand","status":"started","submitted_at":"2024-10-18T19:58:03Z","user_input":"Write a story about Agentic Workflow"}


http://localhost:8086/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c/results
http://localhost:8086/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c/status



