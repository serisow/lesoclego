FROM golang:latest

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    git \
    poppler-utils \
    wv \
    unrtf \
    tidy \
    tesseract-ocr \
    tesseract-ocr-eng \
    ca-certificates \
    wget \
    && rm -rf /var/lib/apt/lists/*