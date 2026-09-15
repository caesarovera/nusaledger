# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.27-alpine AS builder
WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

# Layer dependency terpisah: tidak batal saat kode berubah → build jauh lebih cepat
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/api ./cmd/api

# ---- Runtime stage: tanpa shell, tanpa package manager, bukan root ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/api /api
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
ENV TZ=Asia/Jakarta
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
