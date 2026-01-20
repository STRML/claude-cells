FROM golang:1.25.5

# Set up module cache directory
WORKDIR /go/src/cache

# Copy dependency files first (for better layer caching)
COPY go.mod go.sum ./

# Download dependencies - these get cached in the image
RUN go mod download

# Reset working directory for actual development
WORKDIR /workspace
