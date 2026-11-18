# GamePanel Cloud — Declarative Manifest Examples

This directory contains production-ready declarative YAML manifests demonstrating how to provision multi-tenant boundaries, RBAC roles, runtime classes, and game server instances via the GamePanel Cloud REST API or `pkg/client` SDK.

## Manifests Overview

| File | API Group | Resources | Description |
| :--- | :--- | :--- | :--- |
| [`01-tenant.yaml`](./01-tenant.yaml) | `core.gamepanel.io/v1` | `Tenant` | Creates the `studio-alpha` Organization tenant with CPU, memory, storage, and instance count quotas. |
| [`02-rbac.yaml`](./02-rbac.yaml) | `iam.gamepanel.io/v1` | `TenantRole`, `TenantRoleBinding` | Defines a `game-operator` role within `studio-alpha` and binds it to `User/admin`. |
| [`03-runtimeclass.yaml`](./03-runtimeclass.yaml) | `core.gamepanel.io/v1` | `RuntimeClass` | Registers the default `docker` runtime class with regional `nodeSelector` constraints. |
| [`04-instance-minecraft.yaml`](./04-instance-minecraft.yaml) | `core.gamepanel.io/v1` | `Instance` | Declares a `mc-survival-01` Minecraft server instance with resource requests/limits, TCP ports, environment variables, and injected configuration files (`eula.txt`, `server.properties`). |

---

## Quickstart Guide

### 1. Start the Local Stack

Launch PostgreSQL, Redis, NATS JetStream, `apiserver` (with automatic schema migration and RBAC/admin bootstrap), `controller-manager`, and `node-agent`:

```bash
docker compose up -d --build
```

Verify that the API server is healthy and ready:

```bash
curl -s http://127.0.0.1:8080/readyz
```

### 2. Authenticate and Obtain a Bearer Token

Create a stateful `Session` for the bootstrapped administrator (`admin` / `P@88w0rd`) and export `GP_TOKEN`:

```bash
export GP_SERVER_URL="http://127.0.0.1:8080"
export GP_TOKEN=$(curl -s -X POST "${GP_SERVER_URL}/apis/iam.gamepanel.io/v1/sessions" \
  -H "Content-Type: application/json" \
  -d '{"identifier":"admin","password":"P@88w0rd"}' | jq -r '.token')
```

### 3. Inspect Cluster and Tenant Resources via REST API

```bash
# List compute nodes and runtime classes
curl -s -H "Authorization: Bearer ${GP_TOKEN}" "${GP_SERVER_URL}/apis/core.gamepanel.io/v1/nodes" | jq .
curl -s -H "Authorization: Bearer ${GP_TOKEN}" "${GP_SERVER_URL}/apis/core.gamepanel.io/v1/runtimeclasses" | jq .

# List tenants
curl -s -H "Authorization: Bearer ${GP_TOKEN}" "${GP_SERVER_URL}/apis/core.gamepanel.io/v1/tenants" | jq .
```
