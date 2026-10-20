# Stage 1: Build the Go binary
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates

# Copy dependency definitions and download modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically compiled binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o grutmail .

# Stage 2: Minimal runtime image
FROM alpine:latest

WORKDIR /app

# Add CA certificates for secure TLS connections to remote SMTP / databases
RUN apk --no-cache add ca-certificates

# Copy binary and assets from builder
COPY --from=builder /app/grutmail .
COPY --from=builder /app/web ./web
COPY --from=builder /app/email.tmpl .
COPY --from=builder /app/emails.csv .

# Expose default HTTP port
EXPOSE 8080

# Environment defaults
ENV PORT=8080

CMD ["./grutmail"]
