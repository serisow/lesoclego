package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type DailyFileHandler struct {
    currentFile     *os.File
    currentFileName string
    logDir         string
    mutex          sync.Mutex
    defaultHandler slog.Handler
}

func NewDailyFileHandler(logDir string, opts *slog.HandlerOptions) (*DailyFileHandler, error) {
    // Create logs directory if it doesn't exist
    if err := os.MkdirAll(logDir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create log directory: %w", err)
    }

    h := &DailyFileHandler{
        logDir:         logDir,
        defaultHandler: slog.NewTextHandler(os.Stdout, opts),
    }

    if err := h.rotateIfNeeded(); err != nil {
        return nil, err
    }

    return h, nil
}

func (h *DailyFileHandler) rotateIfNeeded() error {
    h.mutex.Lock()
    defer h.mutex.Unlock()

    fileName := fmt.Sprintf("pipeline-%s.log", time.Now().Format("2006-01-02"))
    filePath := filepath.Join(h.logDir, fileName)

    if fileName == h.currentFileName {
        return nil
    }

    // Close existing file if open
    if h.currentFile != nil {
        h.currentFile.Close()
    }

    // Open new log file
    f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return fmt.Errorf("failed to open log file: %w", err)
    }

    h.currentFile = f
    h.currentFileName = fileName
    return nil
}

func (h *DailyFileHandler) Handle(ctx context.Context, r slog.Record) error {
    if err := h.rotateIfNeeded(); err != nil {
        // If rotation fails, at least log to stdout
        return h.defaultHandler.Handle(ctx, r)
    }

    // Format the log entry
    timeStr := r.Time.Format("2006/01/02 15:04:05.000")
    level := r.Level.String()
    
    // Build attributes string
    var attrs string
    r.Attrs(func(a slog.Attr) bool {
        attrs += fmt.Sprintf(" %s=%v", a.Key, a.Value)
        return true
    })

    // Format the log line
    logLine := fmt.Sprintf("[%s] %-5s %s%s\n", timeStr, level, r.Message, attrs)

    // Write to file
    h.mutex.Lock()
    _, err := h.currentFile.WriteString(logLine)
    h.mutex.Unlock()

    // Also log to default handler (stdout)
    if err2 := h.defaultHandler.Handle(ctx, r); err2 != nil {
        if err == nil {
            err = err2
        }
    }

    return err
}

func (h *DailyFileHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &DailyFileHandler{
        currentFile:     h.currentFile,
        currentFileName: h.currentFileName,
        logDir:         h.logDir,
        mutex:          h.mutex,
        defaultHandler: h.defaultHandler.WithAttrs(attrs),
    }
}

func (h *DailyFileHandler) WithGroup(name string) slog.Handler {
    return &DailyFileHandler{
        currentFile:     h.currentFile,
        currentFileName: h.currentFileName,
        logDir:         h.logDir,
        mutex:          h.mutex,
        defaultHandler: h.defaultHandler.WithGroup(name),
    }
}

func (h *DailyFileHandler) Enabled(ctx context.Context, level slog.Level) bool {
    return h.defaultHandler.Enabled(ctx, level)
}
