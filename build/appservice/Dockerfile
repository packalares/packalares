FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app-service ./cmd/appservice/

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*

# Install helm
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Install kubectl
RUN curl -fsSLO "https://dl.k8s.io/release/$(curl -fsSL https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl && \
    rm kubectl

# Install crictl (for image pull progress tracking)
RUN CRICTL_VERSION=$(curl -fsSL https://api.github.com/repos/kubernetes-sigs/cri-tools/releases/latest | grep '"tag_name"' | sed 's/.*"v\(.*\)".*/\1/') && \
    curl -fsSL "https://github.com/kubernetes-sigs/cri-tools/releases/download/v${CRICTL_VERSION}/crictl-v${CRICTL_VERSION}-linux-amd64.tar.gz" | tar -xz -C /usr/local/bin

COPY --from=builder /app-service /usr/local/bin/app-service

RUN mkdir -p /var/lib/packalares/appservice

ENV APP_DATA_DIR=/var/lib/packalares/appservice
ENV CONTAINER_RUNTIME_ENDPOINT=unix:///run/containerd/containerd.sock
ENV LISTEN_ADDR=:6755

EXPOSE 6755

ENTRYPOINT ["app-service"]
LABEL org.opencontainers.image.description="App service — Helm-based app install/uninstall, chart management, WebSocket metrics, pod status tracking"
