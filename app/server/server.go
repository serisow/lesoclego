package server

import (
	"crypto/tls"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serisow/lesocle/handlers"
	"github.com/serisow/lesocle/plugin_registry"
	"github.com/urfave/negroni"
	"golang.org/x/crypto/acme/autocert"
)

type Config struct {
	Domains      []string
	CertCacheDir string
	HTTPPort     string
	HTTPSPort    string
	IdleTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func SetupRoutes(apiEndpoint string, registry *plugin_registry.PluginRegistry, logger *slog.Logger, db *pgxpool.Pool) *mux.Router {
	r := mux.NewRouter()

	// New route for on-demand pipeline execution
	pipelineHandler := handlers.NewPipelineHandler(apiEndpoint, registry)
	r.HandleFunc("/pipeline/{id}/execute", pipelineHandler.ExecutePipeline).Methods("POST")
	r.HandleFunc("/pipeline/{id}/execution/{execution_id}/status", pipelineHandler.GetExecutionStatus).Methods("GET")
    r.HandleFunc("/pipeline/{id}/execution/{execution_id}/results", pipelineHandler.GetExecutionResults).Methods("GET")

	// Add document search route
    documentSearchHandler := handlers.NewDocumentSearchHandler(db, logger)
    r.Handle("/documents/search", documentSearchHandler).Methods("POST")

	return r
}

// ServeProduction build the server when we operate in a production environment.
func ServeProduction(n *negroni.Negroni) {
	// Configure autocert settings
	autocertManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist("serisow.com", "www.serisow.com"),
		Cache:      autocert.DirCache("../serisow_certs"),
	}

	// Listen for HTTP requests on port 80 in a new goroutine. Use
	// autocertManager.HTTPHandler(nil) as the handler. This will send ACME
	// "http-01" challenge responses as necessary, and 302 redirect all other
	// requests to HTTPS.
	go func() {
		srv := &http.Server{
			Addr:         ":80",
			Handler:      autocertManager.HTTPHandler(nil),
			IdleTimeout:  time.Minute,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		err := srv.ListenAndServe()
		log.Fatal(err)
	}()

	// Configure the TLS config to use the autocertManager.GetCertificate function.
	tlsConfig := &tls.Config{
		GetCertificate:           autocertManager.GetCertificate,
		PreferServerCipherSuites: true,
		CurvePreferences:         []tls.CurveID{tls.X25519, tls.CurveP256},
	}

	srv := &http.Server{
		Addr:         ":443",
		Handler:      n,
		TLSConfig:    tlsConfig,
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	err := srv.ListenAndServeTLS("", "") // Key and cert provided automatically by autocert.
	log.Fatal(err)
}

// ServeDevelopment start the server when we operate in a dev environment.
func ServeDevelopment(s *http.Server) {
	log.Fatal(s.ListenAndServe())
}