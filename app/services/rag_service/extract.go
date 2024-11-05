package rag_service

import (
    "bytes"
    "fmt"
    "log/slog"

    "code.sajari.com/docconv/v2"
    "github.com/ledongthuc/pdf"
)

type DocumentExtractor struct {
    logger *slog.Logger
}

func NewDocumentExtractor(logger *slog.Logger) *DocumentExtractor {
    return &DocumentExtractor{
        logger: logger,
    }
}

func (e *DocumentExtractor) ExtractTextFromPDF(data []byte) (string, error) {
    // Create a reader from the data
    reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
    if err != nil {
        e.logger.Error("Failed to create PDF reader",
            slog.String("error", err.Error()),
            slog.Int("data_size", len(data)))
        return "", fmt.Errorf("failed to create PDF reader: %v", err)
    }

    totalPage := reader.NumPage()
    e.logger.Debug("Starting PDF text extraction",
        slog.Int("total_pages", totalPage))

    var fullText string
    for pageIndex := 1; pageIndex <= totalPage; pageIndex++ {
        page := reader.Page(pageIndex)
        if page.V.IsNull() {
            e.logger.Warn("Null page encountered",
                slog.Int("page_number", pageIndex))
            continue
        }

        text, err := page.GetPlainText(nil)
        if err != nil {
            e.logger.Error("Failed to extract text from page",
                slog.Int("page_number", pageIndex),
                slog.String("error", err.Error()))
            return "", fmt.Errorf("failed to extract text from page %d: %v", pageIndex, err)
        }

        // Log page extraction success with text length
        e.logger.Debug("Extracted text from page",
            slog.Int("page_number", pageIndex),
            slog.Int("text_length", len(text)))

        fullText += text
    }

    if len(fullText) == 0 {
        e.logger.Error("No text extracted from PDF",
            slog.Int("total_pages", totalPage))
        return "", fmt.Errorf("no text content extracted from PDF")
    }

    e.logger.Info("Successfully extracted text from PDF",
        slog.Int("total_pages", totalPage),
        slog.Int("total_text_length", len(fullText)))

    return fullText, nil
}

func (e *DocumentExtractor) ExtractTextFromWord(data []byte) (string, error) {
    e.logger.Debug("Starting Word document text extraction",
        slog.Int("data_size", len(data)))

    mimeType := "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

    result, err := docconv.Convert(bytes.NewReader(data), mimeType, false)
    if err != nil {
        e.logger.Error("Failed to convert Word document",
            slog.String("error", err.Error()),
            slog.Int("data_size", len(data)))
        return "", fmt.Errorf("failed to convert Word document: %v", err)
    }

    if len(result.Body) == 0 {
        e.logger.Error("No text extracted from Word document")
        return "", fmt.Errorf("no text content extracted from Word document")
    }

    e.logger.Info("Successfully extracted text from Word document",
        slog.Int("text_length", len(result.Body)))

    return result.Body, nil
}