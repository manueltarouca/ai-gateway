---
sidebar_position: 2
---

# Gateway Agent

The `gateway-agent` is a standalone Go binary that volunteers run on their machines to contribute compute to the network.

## Installation

### From Source

```bash
cd services/gateway-agent
go build -o gateway-agent ./cmd/
```

### Cross-compile

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o gateway-agent-linux-amd64 ./cmd/

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o gateway-agent-linux-arm64 ./cmd/

# macOS ARM64
GOOS=darwin GOARCH=arm64 go build -o gateway-agent-darwin-arm64 ./cmd/
```

## Usage

```bash
./gateway-agent \
  --gateway=http://your-gateway:9090 \
  --ollama=http://localhost:11434 \
  --name=my-node \
  --poll=2s
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--gateway` | `http://localhost:9090` | Agent API URL |
| `--ollama` | `http://localhost:11434` | Local Ollama URL |
| `--name` | hostname | Node name shown in admin |
| `--models` | auto-detect | Comma-separated models (auto-detects from Ollama if empty) |
| `--keys` | `~/.gateway-agent` | Directory for Ed25519 key pair |
| `--poll` | `2s` | How often to poll for tasks |

## What Happens on First Run

1. **Key generation** — an Ed25519 key pair is created in `~/.gateway-agent/`:
   - `node.pub` — public key (shared with gateway)
   - `node.key` — private key (never leaves your machine, `0600` permissions)

2. **Model detection** — queries Ollama's `/api/tags` to discover available models

3. **Registration** — sends public key and model list to the gateway
   ```
   POST /api/nodes/register
   { "name": "my-node", "public_key": "...", "models": ["gemma4:e4b", ...] }
   ```

4. **Waiting for approval** — the node enters `pending` status and polls until an admin approves it

## Authentication

Every request from the agent to the gateway is signed:

```
X-Node-PublicKey: <base64 public key>
X-Timestamp: <RFC3339 UTC timestamp>
X-Signature: <base64 Ed25519 signature of: method + path + timestamp>
```

The gateway verifies:
- The public key is registered
- The node is active/approved
- The signature is valid
- The timestamp is within 5 minutes of server time

## Task Processing

Once approved, the agent enters a poll loop:

```
every 2s:
  GET /api/tasks/next → task or empty
  if task:
    POST http://localhost:11434/api/chat (local Ollama)
    POST /api/tasks/{id}/result (signed)
  if inference fails:
    POST /api/tasks/{id}/fail (task returns to queue)
```

Failed tasks are automatically requeued for other agents to pick up.

## Heartbeat

Every 30 seconds, the agent sends a heartbeat:

```
POST /api/nodes/heartbeat
```

The first heartbeat after approval transitions the node from `approved` to `active`.
