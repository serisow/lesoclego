# Use PostgreSQL 15 as the base image
FROM postgres:15

# Install build dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    git \
    postgresql-server-dev-15 \
    && rm -rf /var/lib/apt/lists/*

# Clone pgvector
WORKDIR /usr/src
RUN git clone https://github.com/pgvector/pgvector.git

# Build and install pgvector
WORKDIR /usr/src/pgvector
RUN make && make install

# Clean up
RUN apt-get remove -y git build-essential \
    && apt-get autoremove -y \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* /usr/src/pgvector

# Enable pgvector extension
RUN echo "shared_preload_libraries = 'vector'" >> /usr/share/postgresql/postgresql.conf.sample

# Copy initialization scripts
COPY init-scripts/ /docker-entrypoint-initdb.d/

# Set the working directory back to root
WORKDIR /