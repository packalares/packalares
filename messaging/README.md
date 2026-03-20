# Messaging (NATS)

Shared NATS messaging server for marketplace apps. Deployed in the `packalares-platform` namespace.

## Services

| Service | Address | Port | Protocol |
|---------|---------|------|----------|
| NATS client | `nats-svc.packalares-platform` | 4222 | NATS |
| NATS monitoring | `nats-svc.packalares-platform` | 8222 | HTTP |

## Deployment

Deployed automatically by `scripts/setup-platform.sh`. To deploy standalone:

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
bash scripts/setup-platform.sh
```

Or apply the manifest directly:

```bash
kubectl apply -f messaging/nats-deployment.yaml
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NATS_VERSION` | Image tag | `2.10-alpine` |

## Features

- JetStream enabled for persistent messaging
- Max payload: 8MB
- Max connections: 1024
- JetStream limits: 256MB memory, 2GB file storage

## Connecting from Apps

Connect to the NATS URL from any pod in the cluster:

```
nats://nats-svc.packalares-platform:4222
```

Apps can discover the endpoint via the `platform-info` ConfigMap:

```yaml
env:
  - name: NATS_URL
    value: "nats://nats-svc.packalares-platform:4222"
```

## Monitoring

The NATS monitoring endpoint is available at `http://nats-svc.packalares-platform:8222`:

```bash
kubectl run -it --rm nats-check --image=busybox --restart=Never -- \
  wget -qO- http://nats-svc.packalares-platform:8222/healthz
```

## Data

JetStream data is stored on a 5Gi PersistentVolumeClaim (`nats-data`), backed by the default `openebs-hostpath` StorageClass.
