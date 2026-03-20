# Packalares

Self-hosted personal cloud OS. No cloud dependency. Works with IP or domain.

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/packalares/packalares/main/install.sh | bash
```

## What is it?

A personal cloud that runs on your hardware:
- **One-click app installs** from the marketplace
- **File manager** with NAS/Google Drive/S3 support
- **GPU/AI workloads** with NVIDIA support
- **Access from anywhere** via Tailscale or local network
- **No cloud dependency** — your data stays on your hardware

## Architecture

```
Browser → Caddy (reverse proxy, auto HTTPS)
  → Authentik (login, 2FA)
  → Desktop / Settings / Market / Files (Vue.js)
  → Apps (Docker containers via Helm)
  → K3s (Kubernetes)
```

## Config

Everything in one file: `config.yaml`

## Images

All Docker images listed in one file: `images.yaml`
