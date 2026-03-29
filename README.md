# MiniDevOpsHub

A lightweight deployment platform (mini Vercel/Railway) for cloud-native app management and DevOps demos.

## Structure

- `cmd/` — Entrypoints (main.go for control plane, etc.)
- `internal/` — Core business logic (app, worker, deployment, etc.)
- `pkg/` — Shared libraries/utilities
- `api/` — API contracts (OpenAPI/specs, DTOs)

## Getting Started

1. `go run ./cmd/controlplane` — Start backend server
2. Access health check at `http://localhost:8080/healthz`

---

- Control Plane: Go backend, NGINX, Dashboard UI
- Worker Nodes: Docker hosts, managed via SSH
- Dashboard: Frontend (to be added)

---

See project documentation for architecture and feature details.
# MiniDevOpsHub_Test
