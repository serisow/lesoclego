# Use the latest Go image based on Debian
FROM golang:latest

# Install build and runtime dependencies
RUN apt-get update && apt-get install -y \
    git \
    poppler-utils \
    wv \
    unrtf \
    tidy \
    tesseract-ocr \
    tesseract-ocr-eng \
    vim \
    ca-certificates \
 && rm -rf /var/lib/apt/lists/*
