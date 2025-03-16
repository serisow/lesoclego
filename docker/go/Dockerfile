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
    xz-utils \
    fontconfig \
    libfreetype6 \
    && rm -rf /var/lib/apt/lists/*

# Install FFmpeg 6.1.2 (BtbN build with drawtext support)
RUN wget https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2025-03-09-12-54/ffmpeg-n6.1.2-25-g39cac587c4-linux64-gpl-6.1.tar.xz \
    && tar -xf ffmpeg-n6.1.2-*.tar.xz \
    && mv ffmpeg-n6.1.2-*/bin/ffmpeg /usr/local/bin/ \
    && mv ffmpeg-n6.1.2-*/bin/ffprobe /usr/local/bin/ \
    && rm -rf ffmpeg-n6.1.2-*


# Verify FFmpeg and drawtext filter
RUN ffmpeg -filters | grep drawtext

# Install font packages to match Drupal environment
RUN apt-get update && apt-get install -y \
    fonts-dejavu \
    fonts-droid-fallback \
    fonts-freefont-ttf \
    fonts-liberation \
    fonts-opensans \
    && rm -rf /var/lib/apt/lists/*