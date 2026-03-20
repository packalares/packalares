# Object Storage (MinIO)

S3-compatible object storage for marketplace apps. Deployed in the `packalares-platform` namespace.

## Services

| Service | Address | Port | Purpose |
|---------|---------|------|---------|
| S3 API | `s3-svc.packalares-platform` | 9000 | S3-compatible API |
| Console | `s3-svc.packalares-platform` | 9001 | Web management UI |

## Deployment

Deployed automatically by `scripts/setup-platform.sh`. To deploy standalone:

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
bash scripts/setup-platform.sh
```

Or apply the manifest directly (you must create the secret first):

```bash
kubectl create secret generic minio-credentials \
  --namespace packalares-platform \
  --from-literal=root-user="packalares" \
  --from-literal=root-password="$(openssl rand -hex 32)"
kubectl apply -f objectstore/minio-deployment.yaml
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MINIO_VERSION` | Image tag | `RELEASE.2024-06-13T22-53-53Z` |
| `MINIO_ROOT_USER` | Root user name | `packalares` |
| `MINIO_ROOT_PASSWORD` | Root password | auto-generated |

## Connecting from Apps

Apps use standard S3 SDKs or CLI tools. Credentials are stored in the `minio-credentials` secret:

```yaml
env:
  - name: S3_ENDPOINT
    value: "http://s3-svc.packalares-platform:9000"
  - name: S3_ACCESS_KEY
    valueFrom:
      secretKeyRef:
        name: minio-credentials
        namespace: packalares-platform
        key: root-user
  - name: S3_SECRET_KEY
    valueFrom:
      secretKeyRef:
        name: minio-credentials
        namespace: packalares-platform
        key: root-password
```

## Data

Object data is stored on a 20Gi PersistentVolumeClaim (`minio-data`), backed by the default `openebs-hostpath` StorageClass.
