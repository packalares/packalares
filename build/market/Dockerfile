FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /market-backend ./cmd/market/

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /market-backend /usr/local/bin/market-backend
# Copy per-entity sidecars, icons, screenshots, and curation (no catalog.json)
COPY market/charts/ /data/market/charts/
COPY market/icons/ /data/market/icons/
COPY market/screenshots/ /data/market/screenshots/
COPY market/curation.json /data/market/curation.json

ENV MARKET_LISTEN_ADDR=:6756

EXPOSE 6756

ENTRYPOINT ["market-backend"]
LABEL org.opencontainers.image.description="Market backend — self-contained app catalog, chart/icon/screenshot serving"
