# Database (PostgreSQL)

Shared PostgreSQL instance for marketplace apps. Deployed in the `packalares-platform` namespace.

## Service

| Service | Address | Port |
|---------|---------|------|
| PostgreSQL | `postgres-svc.packalares-platform` | 5432 |

Default database: `packalares`
Default user: `packalares`

## Deployment

Deployed automatically by `scripts/setup-platform.sh`. To deploy standalone:

```bash
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
bash scripts/setup-platform.sh
```

Or apply the manifest directly (you must create the secret first):

```bash
kubectl create secret generic postgres-credentials \
  --namespace packalares-platform \
  --from-literal=admin-password="$(openssl rand -hex 32)"
kubectl apply -f database/postgres-deployment.yaml
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `POSTGRES_VERSION` | Image tag | `16-alpine` |
| `POSTGRES_PASSWORD` | Admin password | auto-generated |

## Connecting from Apps

Apps can retrieve the connection string from the `postgres-credentials` secret:

```yaml
env:
  - name: DATABASE_URL
    valueFrom:
      secretKeyRef:
        name: postgres-credentials
        namespace: packalares-platform
        key: connection-string
```

Or connect directly to `postgres-svc.packalares-platform:5432` with the password from the secret.

## Configuration

The `pg_hba.conf` is stored in a ConfigMap (`postgres-config`) and allows connections from the pod and cluster CIDRs using `scram-sha-256` authentication.

## Data

PostgreSQL data is stored on a 10Gi PersistentVolumeClaim (`postgres-data`), backed by the default `openebs-hostpath` StorageClass.
