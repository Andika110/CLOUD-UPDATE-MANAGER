# Multi-Stage Build for Quinos Cloud Update Manager
FROM golang:alpine AS builder

WORKDIR /build

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o quinos-cloud-server ./cmd/server

# Minimal Runtime Image
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Jakarta

COPY --from=builder /build/quinos-cloud-server /app/quinos-cloud-server
COPY --from=builder /build/web /app/web

RUN mkdir -p /app/data/packages

EXPOSE 8080

CMD ["/app/quinos-cloud-server", "-port", ":8080", "-db", "/app/data/quinos_cloud.db", "-storage", "/app/data/packages", "-html", "/app/web/index.html"]
