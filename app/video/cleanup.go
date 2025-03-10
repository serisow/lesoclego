package video

import (
    "log/slog"
    "os"
    "path/filepath"
    "time"
)

// VideoCleanupService handles the removal of old video files
type VideoCleanupService struct {
    logger        *slog.Logger
    retentionDays int
}

// NewVideoCleanupService creates a new cleanup service
func NewVideoCleanupService(logger *slog.Logger, retentionDays int) *VideoCleanupService {
    return &VideoCleanupService{
        logger:        logger,
        retentionDays: retentionDays,
    }
}

// StartCleanupSchedule begins regular cleanup of old video files
func (s *VideoCleanupService) StartCleanupSchedule(interval time.Duration) {
    ticker := time.NewTicker(interval)
    
    go func() {
        for range ticker.C {
            s.PerformCleanup()
        }
    }()
    
    s.logger.Info("Video cleanup service started",
        slog.Int("retention_days", s.retentionDays),
        slog.Duration("interval", interval))
}

// PerformCleanup removes videos older than the retention period
func (s *VideoCleanupService) PerformCleanup() {
    basePath := filepath.Join("storage", "pipeline", "videos")
    
    cutoffTime := time.Now().AddDate(0, 0, -s.retentionDays)
    
    // Walk through all video directories
    err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        
        // Skip directories
        if info.IsDir() {
            return nil
        }
        
        // Check file extension
        if filepath.Ext(path) != ".mp4" && filepath.Ext(path) != ".mov" && filepath.Ext(path) != ".webm" {
            return nil
        }
        
        // Check if file is older than retention period
        if info.ModTime().Before(cutoffTime) {
            s.logger.Info("Removing old video file",
                slog.String("path", path),
                slog.Time("modified_time", info.ModTime()),
                slog.Time("cutoff_time", cutoffTime))
                
            if err := os.Remove(path); err != nil {
                s.logger.Error("Failed to remove video file",
                    slog.String("path", path),
                    slog.String("error", err.Error()))
            }
        }
        
        return nil
    })
    
    if err != nil {
        s.logger.Error("Error during video cleanup",
            slog.String("error", err.Error()))
    }
}