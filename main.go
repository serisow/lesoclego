package main

import (
	"log"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/serisow/lesocle/action_step"
	"github.com/serisow/lesocle/config"
	"github.com/serisow/lesocle/llm_step"
	"github.com/serisow/lesocle/logging"
	"github.com/serisow/lesocle/pipeline"
	"github.com/serisow/lesocle/pipeline/step"
	"github.com/serisow/lesocle/plugin_registry"
	"github.com/serisow/lesocle/scheduler"
	"github.com/serisow/lesocle/search_step"
	"github.com/serisow/lesocle/server"

	//"github.com/serisow/lesocle/services/action_service"
	"github.com/serisow/lesocle/services/action_service"
	"github.com/serisow/lesocle/services/llm_service"

	"github.com/urfave/negroni"
)

func main() {
	cfg := config.Load()

	// Initialize the logger
	logger, err := initLogger()
    if err != nil {
        log.Fatalf("Failed to initialize logger: %v", err)
    }

	// Initialize PluginRegistry
	registry := plugin_registry.NewPluginRegistry()
	registerStepTypes(registry, logger)

	// Initialize scheduler with PluginRegistry
	s := scheduler.New(cfg.APIEndpoint, cfg.CheckInterval, registry)
	go s.Start()

    // Start the execution store cleanup
    executionResultRetention := 24 * time.Hour // Retain results for 24 hours
    cleanupInterval := 1 * time.Hour           // Run cleanup every hour
    pipeline.StartExecutionStoreCleanup(executionResultRetention, cleanupInterval)

	// Initialize server
	r := server.SetupRoutes(cfg.APIEndpoint, registry)
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

func registerStepTypes(registry *plugin_registry.PluginRegistry, logger *slog.Logger) {
	// Register the Step Types
	registry.RegisterStepType("llm_step", func() step.Step {
		return &llm_step.LLMStepImpl{
			LLMServiceInstance: nil, // This will be set later based on configuration
		}
	})
	registry.RegisterStepType("action_step", func() step.Step {
		return &action_step.ActionStepImpl{}
	})
	registry.RegisterStepType("google_search", func() step.Step {
        return &search_step.GoogleSearchStepImpl{}
    })

	// Register the LLM Services
	registry.RegisterLLMService("openai", llm_service.NewOpenAIService(logger))
	registry.RegisterLLMService("openai_image", llm_service.NewOpenAIImageService(logger))
	registry.RegisterLLMService("anthropic", llm_service.NewAnthropicService(logger))
	registry.RegisterLLMService("gemini", llm_service.NewGeminiService(logger))

	// Register Action services
	
	// This one is just a placeholder; will be removed
	registry.RegisterActionService("process_data_action", &action_service.ProcessDataActionService{})
	registry.RegisterActionService("post_tweet", action_service.NewPostTweetActionService(logger))
	registry.RegisterActionService("send_sms", action_service.NewSendSMSActionService(logger))
	registry.RegisterActionService("generic_webhook", action_service.NewGenericWebhookActionService(logger))
	//// THESE ARE DRUPAL SIDE ACTION SERVICE, WILL ONLY RUN ON DRUPAL
	//registry.RegisterActionService("create_article_action", &action_service.CreateArticleAction{})
	//registry.RegisterActionService("update_entity_action", &action_service.UpdateEntityAction{})
	//registry.RegisterActionService("fetch_taxonomy", &action_service.FetchTaxonomyAction{})

}

func initLogger() (*slog.Logger, error) {
    // Configure log directory - you might want to make this configurable
    logDir := filepath.Join("logs", "pipeline")

    // Create daily file handler
    fileHandler, err := logging.NewDailyFileHandler(logDir, &slog.HandlerOptions{
        Level: slog.LevelDebug,
        ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
            // You can customize attribute handling here if needed
            return a
        },
    })
    if err != nil {
        return nil, err
    }

    // Create logger with the custom handler
    logger := slog.New(fileHandler)

    return logger, nil
}