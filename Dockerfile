# Multi-stage Dockerfile to build and run Go Wasm Host and React Frontend

# --- STAGE 1: Build React Frontend ---
FROM node:20-bookworm AS frontend-builder
WORKDIR /app/frontend

# Install node dependencies
COPY frontend/package*.json ./
RUN npm install

# Build static assets
COPY frontend/ ./
RUN npm run build

# --- STAGE 2: Build Go Host Binary ---
FROM golang:1.25-bookworm AS backend-builder

# Install GCC for compiling Wasmtime CGO bindings
RUN apt-get update && apt-get install -y \
    build-essential \
    gcc \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY main.go ./
COPY db/ ./db/
COPY sandbox/ ./sandbox/
COPY compiler/ ./compiler/

# Build backend
ENV CGO_ENABLED=1
RUN go build -o wasm-host main.go

# --- STAGE 3: Production Runtime Environment ---
FROM golang:1.25-bookworm

# Install runtime compiler tools and Rust toolchain
RUN apt-get update && apt-get install -y \
    curl \
    build-essential \
    gcc \
    && rm -rf /var/lib/apt/lists/*

# Pre-install Rust and stable wasm32-wasip1 target
ENV RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=/usr/local/cargo \
    PATH=/usr/local/cargo/bin:$PATH
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain stable
RUN rustup target add wasm32-wasip1

WORKDIR /app

# Copy binary, compiled static assets, and template files
COPY --from=backend-builder /app/wasm-host ./
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist
COPY compiler/template/ ./compiler/template/

# Expose HTTP port
EXPOSE 8080

# Environment configurations
ENV PORT=8080

# Run host
CMD ["./wasm-host"]

