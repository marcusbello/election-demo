# ---- Build stage ----
FROM golang:1.22-alpine AS builder
WORKDIR /app

# Install git (in case dependencies are added later)
RUN apk add --no-cache git

# Copy Go source
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build static binary
RUN go build -o server main.go

# ---- Final stage ----
FROM alpine:3.20
WORKDIR /app

# Copy binary & assets
COPY --from=builder /app/server .
COPY templates/ templates/
COPY static/ static/
COPY data/ data/

# Expose port
EXPOSE 8080

# Run server
CMD ["./server"]
