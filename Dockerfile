FROM golang:1.25-alpine AS builder

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=$(cat VERSION 2>/dev/null || echo 0.1.0)" \
    -o /sentinel ./cmd/sentinel/

# Final image — scratch for minimal size
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /sentinel /sentinel

EXPOSE 9200

ENTRYPOINT ["/sentinel"]
