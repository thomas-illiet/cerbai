# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /src

# Download dependencies first for layer caching.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.buildDate=${BUILD_DATE}" \
    -trimpath \
    -o /cerbai .

# ── Final stage ───────────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S cerbai && adduser -S cerbai -G cerbai

WORKDIR /app

COPY --from=builder /cerbai /app/cerbai

USER cerbai

EXPOSE 8085

ENTRYPOINT ["/app/cerbai"]
