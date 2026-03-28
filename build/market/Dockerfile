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

RUN mkdir -p /etc/packalares

ENV MARKET_URL=https://market.olares.com
ENV MARKET_LISTEN_ADDR=:6756

EXPOSE 6756

ENTRYPOINT ["market-backend"]
LABEL org.opencontainers.image.description="Market backend — app catalog, chart sync, icon/screenshot serving, Olares marketplace integration"
