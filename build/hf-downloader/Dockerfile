FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /hf-downloader ./cmd/hf-downloader/

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /hf-downloader /usr/local/bin/hf-downloader

ENTRYPOINT ["hf-downloader"]
LABEL org.opencontainers.image.description="HuggingFace model downloader for vLLM"
