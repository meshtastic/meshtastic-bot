FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -a -installsuffix cgo \
    -ldflags '-s -w -extldflags "-static"' \
    -o meshtastic-bot .

FROM alpine:latest

# Add ca-certificates and wget for HTTPS and health checks
RUN apk --no-cache add ca-certificates wget

WORKDIR /app

COPY --from=builder /build/meshtastic-bot .
COPY config.yaml /app/config.yaml
COPY faq.yaml /app/faq.yaml

ENV HEALTHCHECK_PORT=8080
ENV CONFIG_PATH=/app/config.yaml
ENV FAQ_PATH=/app/faq.yaml

# Expose health check port
EXPOSE ${HEALTHCHECK_PORT}

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:${HEALTHCHECK_PORT}/health || exit 1

# Run the application
ENTRYPOINT ["/app/meshtastic-bot"]
