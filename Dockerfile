# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

COPY . .

ARG TARGETARCH

RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags='-s -w' -o /out/mcp-searxng ./

# Runtime stage
FROM alpine:3.21

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata dumb-init

COPY --from=builder /out/mcp-searxng /app/mcp-searxng

ENV MCP_HTTP_PORT=8080

EXPOSE 8080

ENTRYPOINT ["dumb-init", "--", "/app/mcp-searxng"]
