FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /monitor-server ./cmd/monitor

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /monitor-server /usr/local/bin/monitor-server
EXPOSE 8000
ENV PROMETHEUS_URL=http://prometheus.kubesphere-monitoring-system.svc:9090
ENV PORT=8000
ENTRYPOINT ["monitor-server"]
LABEL org.opencontainers.image.description="Monitoring server — system metrics collection, Prometheus queries, Loki log proxy, KVRocks metrics publishing"
