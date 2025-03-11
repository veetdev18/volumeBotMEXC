FROM golang:1.20-alpine AS builder

WORKDIR /app

# Install Node.js and npm
RUN apk add --no-cache nodejs npm

# Install CCXT
RUN npm install ccxt

# Copy Go module files and download dependencies
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o volumebot ./cmd/bot

# Create runtime image
FROM alpine:3.18

# Install Node.js and npm
RUN apk add --no-cache nodejs npm

# Install CCXT
RUN npm install -g ccxt

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/volumebot .
COPY --from=builder /app/.env* ./

# Create a non-root user to run the application
RUN adduser -D appuser
USER appuser

# Command to run the application
CMD ["./volumebot"] 