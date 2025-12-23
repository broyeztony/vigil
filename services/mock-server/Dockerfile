# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY services/mock-server/ ./services/mock-server/
COPY internal/ ./internal/

# Build the application from module root
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o mock-server ./services/mock-server

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates wget

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/mock-server .
RUN chmod +x ./mock-server

EXPOSE 8080

ENV PORT=8080

CMD ["./mock-server"]

