# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY services/discovery-service/ ./services/discovery-service/
COPY internal/ ./internal/

# Build the application from module root
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o discovery ./services/discovery-service/cmd/discovery

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates postgresql-client wget

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/discovery .

EXPOSE 8081

CMD ["./discovery", "run"]

