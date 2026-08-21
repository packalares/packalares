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

# Install helm — pinned (latest 3.x; do NOT bump to helm 4.x without testing,
# it is a breaking major). Was: get-helm-3 script (unpinned "latest 3.x").
RUN HELM_VERSION=v3.21.4 && \
    curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz" | tar -xz -C /tmp && \
    install -o root -g root -m 0755 /tmp/linux-amd64/helm /usr/local/bin/helm && \
    rm -rf /tmp/linux-amd64

# Install kubectl — pinned to the cluster's minor (k3s v1.29). Keep in lockstep
# with the cluster on upgrade (kubectl supports only +/-1 minor skew).
# Was: unpinned fetch of dl.k8s.io/release/stable.txt (currently v1.36, a 7-minor
# skew against a 1.29 cluster).
RUN KUBECTL_VERSION=v1.29.15 && \
    curl -fsSLO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" && \
    install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl && \
    rm kubectl

# Install crictl (for image pull progress tracking) and ctr (for content GC).
# crictl pinned to the cluster minor. Was: unpinned GitHub "latest" release.
RUN CRICTL_VERSION=1.29.0 && \
    curl -fsSL "https://github.com/kubernetes-sigs/cri-tools/releases/download/v${CRICTL_VERSION}/crictl-v${CRICTL_VERSION}-linux-amd64.tar.gz" | tar -xz -C /usr/local/bin

# Install ctr (containerd CLI for content store garbage collection)
RUN CTR_VERSION=1.7.24 && \
    curl -fsSL "https://github.com/containerd/containerd/releases/download/v${CTR_VERSION}/containerd-${CTR_VERSION}-linux-amd64.tar.gz" | tar -xz -C /usr/local bin/ctr

COPY --from=builder /app-service /usr/local/bin/app-service
COPY --from=builder /src/deploy/charts/vllm-model /usr/share/packalares/charts/vllm-model

RUN mkdir -p /var/lib/packalares/appservice

ENV APP_DATA_DIR=/var/lib/packalares/appservice
ENV CONTAINER_RUNTIME_ENDPOINT=unix:///run/containerd/containerd.sock
ENV LISTEN_ADDR=:6755

EXPOSE 6755

ENTRYPOINT ["app-service"]
LABEL org.opencontainers.image.description="App service — Helm-based app install/uninstall, chart management, WebSocket metrics, pod status tracking"
