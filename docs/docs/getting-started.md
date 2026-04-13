---
sidebar_position: 1
---

# Getting Started

AI Gateway is a community-governed, OpenAI-compatible API gateway backed by open-weight models. It uses LiteLLM as the gateway layer with pluggable inference backends.

## Prerequisites

- Docker / Podman with Docker Compose
- [Ollama](https://ollama.com) installed and running
- Models pulled:
  ```bash
  ollama pull gemma4:e4b
  ollama pull qwen3.5:9B
  ```

## Quick Start

### 1. Clone the repository

```bash
git clone https://github.com/manueltarouca/ai-gateway.git
cd ai-gateway
```

### 2. Start the gateway stack

```bash
docker-compose up -d
```

This starts three services:
- **Postgres** (port 5432) — state and task queue
- **agent-api** (port 9090) — community node management and task queue API
- **LiteLLM** (port 4000) — OpenAI-compatible gateway

Wait ~30 seconds for LiteLLM to initialize on first run (database migrations).

### 3. Verify

```bash
curl -s -H "Authorization: Bearer sk-local-dev-master-key" \
  http://localhost:4000/health
```

### 4. Make a request

```bash
curl -X POST http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer sk-local-dev-master-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma4",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 300
  }'
```

### 5. Run the smoke test

```bash
bash tests/smoke-test.sh
```

## Optional: Web UI

A LibreChat frontend can be set up to provide a chat interface:

```bash
cd frontend
git clone https://github.com/danny-avila/LibreChat.git
cd LibreChat
cp .env.example .env
echo 'AI_GATEWAY_KEY=sk-local-dev-master-key' >> .env
docker-compose up -d
```

Then visit http://localhost:3080 and register an account.

:::note Podman users
Comment out the `extra_hosts` block in LibreChat's `docker-compose.yml` — Podman uses `host.containers.internal` instead of `host-gateway`.
:::

## Stopping

```bash
docker-compose down        # stops containers, keeps data
docker-compose down -v     # stops containers AND deletes volumes
```
