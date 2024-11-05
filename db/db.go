package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect() (*pgxpool.Pool, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	var pool *pgxpool.Pool
	var err error
	maxRetries := 10
	retryDelay := time.Second * 10

	for i := 0; i < maxRetries; i++ {
		config, err := pgxpool.ParseConfig(dbURL)
		if err != nil {
			return nil, fmt.Errorf("unable to parse DATABASE_URL: %v", err)
		}

		pool, err = pgxpool.NewWithConfig(context.Background(), config)
		if err == nil {
			err = pool.Ping(context.Background())
			if err == nil {
				log.Println("Successfully connected to the database")
				break
			}
		}

		log.Printf("Failed to connect to the database (attempt %d/%d): %v", i+1, maxRetries, err)
		if i < maxRetries-1 {
			log.Printf("Retrying in %v...", retryDelay)
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to the database after %d attempts: %v", maxRetries, err)
	}

	// Enable pgvector extension
	_, err = pool.Exec(context.Background(), "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		return nil, fmt.Errorf("unable to create vector extension: %v", err)
	}

	return pool, nil
}