---
sidebar_position: 3
---

# Admin API

The agent-api exposes admin endpoints for managing community nodes. All admin endpoints require the master key.

## Authentication

Pass the master key as a Bearer token:

```bash
curl -H "Authorization: Bearer sk-local-dev-master-key" \
  http://localhost:9090/api/admin/nodes
```

## Endpoints

### List Nodes

```
GET /api/admin/nodes
GET /api/admin/nodes?status=pending
GET /api/admin/nodes?status=approved
GET /api/admin/nodes?status=active
GET /api/admin/nodes?status=suspended
```

**Response:**

```json
[
  {
    "id": "560f8c17-...",
    "name": "volunteer-node-1",
    "public_key": "mRm1B3WYnn7S...",
    "models": ["gemma4:e4b", "qwen3.5:9B"],
    "status": "pending",
    "kudos": 0,
    "created_at": "2026-04-13T23:21:59Z"
  }
]
```

### Approve a Node

Transitions a node from `pending` to `approved`. The node will become `active` on its next heartbeat.

```
POST /api/admin/nodes/{id}/approve
```

**Response:**

```json
{"status": "approved"}
```

### Suspend a Node

Immediately prevents a node from receiving tasks. Can be applied to any non-suspended node.

```
POST /api/admin/nodes/{id}/suspend
```

**Response:**

```json
{"status": "suspended"}
```

## Typical Workflow

```bash
# 1. Check for pending nodes
curl -s -H "Authorization: Bearer sk-local-dev-master-key" \
  "http://localhost:9090/api/admin/nodes?status=pending" | python3 -m json.tool

# 2. Approve a node
curl -s -X POST -H "Authorization: Bearer sk-local-dev-master-key" \
  http://localhost:9090/api/admin/nodes/<node-id>/approve

# 3. Verify it's active (after next heartbeat)
curl -s -H "Authorization: Bearer sk-local-dev-master-key" \
  "http://localhost:9090/api/admin/nodes?status=active" | python3 -m json.tool

# 4. Suspend if needed
curl -s -X POST -H "Authorization: Bearer sk-local-dev-master-key" \
  http://localhost:9090/api/admin/nodes/<node-id>/suspend
```

## Node States

| State | Description | Can poll tasks? | Transitions to |
|-------|-------------|-----------------|----------------|
| `pending` | Just registered, awaiting admin review | No | `approved` |
| `approved` | Admin approved, waiting for first heartbeat | No | `active` |
| `active` | Online and processing tasks | Yes | `suspended` |
| `suspended` | Disabled by admin | No | — |
