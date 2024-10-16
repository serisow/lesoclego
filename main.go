package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/serisow/lesocle/config"
	"github.com/serisow/lesocle/pipeline"
	"github.com/serisow/lesocle/pipeline/llm_service"
	"github.com/serisow/lesocle/scheduler"
	"github.com/serisow/lesocle/server"

	"github.com/urfave/negroni"
)

func main() {
	cfg := config.Load()
	// Initialize the logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Initialize PluginRegistry
	registry := pipeline.NewPluginRegistry()
	registerStepTypes(registry, logger)

	// Initialize scheduler with PluginRegistry
	s := scheduler.New(cfg.APIEndpoint, cfg.CheckInterval, registry)
	go s.Start()

	// Initialize server
	r := server.SetupRoutes()
	n := setupNegroni(r)

	if cfg.Environment == "production" {
		server.ServeProduction(n)
	} else {
		srv := &http.Server{
			Addr:         ":" + cfg.HTTPPort,
			Handler:      n,
			IdleTimeout:  time.Minute,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		server.ServeDevelopment(srv)
	}
}

func setupNegroni(r *mux.Router) *negroni.Negroni {
	n := negroni.New()

	// Add middleware here
	n.Use(negroni.NewRecovery())
	n.Use(negroni.NewLogger())

	// Add your custom middleware here if needed

	n.UseHandler(r)
	return n
}

func registerStepTypes(registry *pipeline.PluginRegistry, logger *slog.Logger) {
	// Register the Step Types
	registry.RegisterStepType("llm_step", func() pipeline.Step {
		return &pipeline.LLMStepImpl{
			LLMServiceInstance: nil, // This will be set later based on configuration
		}
	})
	registry.RegisterStepType("action_step", func() pipeline.Step {
		return &pipeline.ActionStepImpl{}
	})
	registry.RegisterStepType("google_search", func() pipeline.Step {
        return &pipeline.GoogleSearchStepImpl{}
    })

	// Register the LLM Services
	registry.RegisterLLMService("openai", llm_service.NewOpenAIService(logger))
	registry.RegisterLLMService("openai_image", llm_service.NewOpenAIImageService(logger)) // use dall-e model for now
	registry.RegisterLLMService("anthropic", llm_service.NewAnthropicService(logger))
	registry.RegisterLLMService("gemini", llm_service.NewGeminiService(logger))

	// Register Action services
	registry.RegisterActionService("create_article_action", &pipeline.CreateArticleAction{})
	registry.RegisterActionService("update_entity_action", &pipeline.UpdateEntityAction{})
}
