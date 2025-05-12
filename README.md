## Project Overview

Lesocle-Go is a sophisticated pipeline execution system designed for automating complex content generation workflows. It's built in Go with a modular architecture that orchestrates various processing steps, ranging from AI model interactions to search operations, content generation, and social media publishing.

The system can be thought of as a flexible workflow engine that connects different services and capabilities through a plugin-based architecture, making it highly extensible for diverse content automation needs.

## Core Architecture

### Pipeline System

The heart of the application is a pipeline execution engine that processes a sequence of steps defined in pipeline configurations. Each pipeline contains multiple steps that are executed in order, with results from each step available to subsequent steps.

```
[Pipeline Configuration] → [Pipeline Engine] → [Step 1] → [Step 2] → ... → [Step N] → [Results]
```

### Plugin-Based Architecture

The system follows a plugin-based architecture where:
1. **Step Types** define different operations (LLM interactions, actions, searches, etc.)
2. **Services** provide integrations with external systems (OpenAI, Anthropic, Google, social media platforms)
3. **Registry** manages all available steps and services

This design allows for easy extension without modifying core code.

## Key Components

### 1. Pipeline Core

**Pipeline Execution Engine** (`pipeline/pipeline.go`):
- Orchestrates step execution
- Manages the pipeline context (data passing between steps)
- Handles errors and result reporting
- Tracks execution status

**Pipeline Types** (`pipeline_type/type.go`):
- Defines data structures for pipelines and steps
- Contains configuration types for different step types
- Provides context management for data sharing

**Plugin Registry** (`plugin_registry/plugin_registry.go`):
- Central repository for all available components
- Manages registration and retrieval of steps and services
- Enables dynamic loading of components

### 2. Step Implementations

Steps are the building blocks of pipelines, each performing a specific function:

**LLM Step** (`llm_step/llm_step.go`):
- Handles interactions with language models (OpenAI, Anthropic, Gemini)
- Processes prompts with dynamic content replacement
- Stores responses in the pipeline context

**Action Step** (`action_step/action_step.go`):
- Executes actions both on Go-side and Drupal-side
- For Go-side: Performs actions directly using action services
- For Drupal-side: Prepares data to be executed by Drupal

**Search Steps**:
- `search_step/google_search_step.go`: Performs Google Custom Search and extracts content
- `search_step/news_api_search_step.go`: Retrieves and processes news articles

**Social Media Step** (`social_media_step/social_media_step.go`):
- Generates optimized content for multiple platforms (Twitter, LinkedIn, Facebook)
- Handles platform-specific formatting and length constraints

**Upload Steps**:
- `upload_step/upload_image_step.go`: Handles image download and storage
- `upload_step/upload_audio_step.go`: Manages audio file processing

### 3. Service Layer

**LLM Services** (`services/llm_service/`):
- Abstract interface for language model interactions
- Implementations for different providers:
  - `openai.go`: OpenAI API integration with retry logic
  - `anthropic.go`: Anthropic Claude API integration
  - `gemini.go`: Google Gemini integration
  - `elevenlabs.go`: Text-to-speech generation with ElevenLabs
  - `aws_polly.go`: Alternative text-to-speech using AWS Polly

**Action Services** (`services/action_service/`):
- Interface for executing various actions
- Implementations include:
  - Social media posting (Twitter, LinkedIn, Facebook)
  - SMS sending
  - News image generation
  - Webhook integration

### 4. Infrastructure

**Scheduler** (`scheduler/scheduler.go`):
- Manages pipeline scheduling (one-time and recurring)
- Checks for pipelines that need execution
- Handles execution coordination and failure management

**Server** (`server/server.go`):
- HTTP server for on-demand pipeline execution
- API endpoints for execution status and results
- File serving for generated media

**Main Application** (`main.go`):
- Application entry point
- Initializes components and registers plugins
- Sets up server and scheduler processes

## Data Flow

1. A pipeline configuration is defined with multiple steps
2. The scheduler or an API request triggers pipeline execution
3. The pipeline engine executes each step in sequence:
   - LLM steps interact with AI models
   - Search steps retrieve external data
   - Action steps perform operations or prepare data
   - Media steps generate content
4. Each step stores its output in the pipeline context
5. The pipeline tracks execution status and results
6. Results are reported back to the calling system

## Component Relationships

```
┌─────────────────────────────────────────────────────────────┐
│                       Main Application                      │
└───────────────────────────┬─────────────────────────────────┘
                           │
          ┌────────────────┼────────────────┐
          │                │                │
┌─────────▼──────────┐ ┌───▼───────────┐ ┌──▼───────────────┐
│      Server        │ │   Scheduler   │ │  Plugin Registry  │
└─────────┬──────────┘ └───────┬───────┘ └──┬───────────────┘
          │                    │            │
          └────────────┬───────┘            │
                      │                     │
               ┌──────▼─────────┐           │
               │ Pipeline Engine◄───────────┘
               └──────┬─────────┘
                      │
┌─────────────────────┼─────────────────────────────────────┐
│                     │                                     │
│     ┌───────────────┼───────────────┐                     │
│     │               │               │                     │
│  ┌──▼──┐        ┌───▼───┐       ┌──▼───┐                 │
│  │LLM  │        │Search │       │Action│                 │
│  │Steps│        │Steps  │       │Steps │                 │
│  └──┬──┘        └───┬───┘       └──┬───┘                 │
│     │               │              │                     │
│  ┌──▼──────────────▼──────────────▼───┐                 │
│  │          Pipeline Context           │                 │
│  └────────────────────────────────────┘                 │
│                                                         │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│                   Service Layer                         │
│                                                         │
│  ┌───────────────┐  ┌──────────────┐ ┌───────────────┐  │
│  │  LLM Services │  │Action Services│ │Media Services │  │
│  │  - OpenAI     │  │- Social Media │ │- Image        │  │
│  │  - Anthropic  │  │- SMS          │ │- Audio        │  │
│  │  - Gemini     │  │- Webhooks     │ │               │  │
│  └───────────────┘  └──────────────┘ └───────────────┘  │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

## Execution Process

1. **Initialization**:
   - Load configuration
   - Initialize plugin registry
   - Register step types and services
   - Start scheduler and server

2. **Pipeline Execution**:
   - Triggered by scheduler or API request
   - Pipeline steps sorted by weight
   - Each step is executed in sequence
   - Context passed between steps for data sharing

3. **Step Processing**:
   - Step implementation retrieved from registry
   - Step configuration applied
   - Step execution with context access
   - Results stored in pipeline context

4. **Result Management**:
   - Execution status tracked
   - Results stored for retrieval
   - Notifications sent when configured

## Key Files and Their Roles

1. **`main.go`**: 
   - Application entry point
   - Initializes components
   - Configures and starts server and scheduler

2. **`pipeline/pipeline.go`**: 
   - Core execution engine
   - Step execution orchestration
   - Error handling and reporting

3. **`pipeline_type/type.go`**: 
   - Core data structures
   - Context management
   - Step configuration types

4. **`plugin_registry/plugin_registry.go`**: 
   - Component registration system
   - Service and step type management

5. **`llm_step/llm_step.go`**: 
   - LLM interaction implementation
   - Prompt processing
   - Response handling

6. **`action_step/action_step.go`**: 
   - Action execution logic
   - Drupal/Go execution location handling

7. **`services/llm_service/llm_service.go`**: 
   - LLM service interface definition
   - Common utility functions

8. **`services/action_service/action_service.go`**: 
   - Action service interface
   - Base implementation

9. **`search_step/google_search_step.go`**: 
   - Google search implementation
   - Result formatting

10. **`social_media_step/social_media_step.go`**: 
    - Social media content generation
    - Platform-specific formatting

11. **`video/service.go`**: 
    - Video generation orchestration
    - Media file management

12. **`scheduler/scheduler.go`**: 
    - Scheduling logic
    - Pipeline trigger management

13. **`server/server.go`**: 
    - HTTP server configuration
    - Route setup

## Extension Points

The system is designed for extensibility through several mechanisms:

1. **New Step Types**: Register additional step types for new capabilities
2. **New LLM Services**: Add integrations with other AI providers
3. **New Action Services**: Implement additional platform-specific actions
4. **Custom Media Processing**: Enhance media generation capabilities

## Interaction Model

```
┌─────────────────┐            ┌────────────────┐
│ External APIs │            │ Drupal Backend │
└───────┬───────┘            └────────┬───────┘
        │                            │
        │HTTP                        │HTTP
        │                            │
┌───────▼────────────────────────────▼───────┐
│                                            │
│              Lesocle-Go System             │
│                                            │
│  ┌──────────────────────────────────────┐  │
│  │            Pipeline Engine           │  │
│  └──────────────────────────────────────┘  │
│                                            │
│  ┌─────────┐ ┌────────┐ ┌───────────────┐  │
│  │ LLM     │ │ Search │ │ Social Media  │  │
│  │ Steps   │ │ Steps  │ │ Steps         │  │
│  └─────────┘ └────────┘ └───────────────┘  │
│                                            │
│  ┌─────────┐ ┌────────┐                    │
│  │ Action  │ │ Upload │                    │
│  │ Steps   │ │ Steps  │                    │
│  └─────────┘ └────────┘                    │
│                                            │
└────────────────────────────────────────────┘
```

This architecture allows the Lesocle-Go system to serve as a powerful content automation platform, connecting multiple AI capabilities, external services, and automated content creation into cohesive workflows.


# Useful commands

docker compose down && cd app/ && GOOS=linux go build -o lesoclego && cd .. && docker compose up


EXAMPLE call one-time execution:

curl -X POST http://lesoclego-dev.sa/pipeline/test_first_on_demand/execute \
     -H "Content-Type: application/json" \
     -d '{"user_input": "Analyze the impact of artificial intelligence on job markets"}'


     {"execution_id":"aa90167b-4f2a-4915-8e30-f50e094ab11c","links":{"results":"/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c/results","self":"/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c","status":"/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c/status"},"pipeline_id":"test_first_on_demand","status":"started","submitted_at":"2024-10-18T19:58:03Z","user_input":"Write a story about Agentic Workflow"}


http://lesoclego-dev.sa/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c/results
http://lesoclego-dev.sa/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c/status