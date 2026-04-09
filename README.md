# AI Gateway — Local Development MVP

A community-governed AI gateway backed by open-weight models, serving an OpenAI-compatible API through LiteLLM with identity, quota, audit, and privacy enforcement at the gateway layer. See `docs/VISION.md` for the full project vision.

## Architecture

```
LibreChat (localhost:3080)       ← web UI
  │
  ▼
LiteLLM Proxy (localhost:4000)   ← virtual keys, rate limits, audit logging
  │
  ▼
Ollama (localhost:11434)         ← open-weight model backends
  ├── qwen3.5 (qwen3.5:9B)
  └── gemma4 (gemma4:e4b)
```

## Prerequisites

- Docker / Podman with Docker Compose
- Ollama installed and running (`ollama serve`)
- Models pulled: `ollama pull qwen3.5:9B` and `ollama pull gemma4:e4b`

## Quick Start

### 1. Start the gateway stack (Postgres + LiteLLM)

```bash
docker-compose up -d

# Wait ~15 seconds for LiteLLM to initialize, then verify
curl -s -H "Authorization: Bearer sk-local-dev-master-key" http://localhost:4000/health
```

### 2. Start the web UI (LibreChat)

```bash
cd frontend/LibreChat
cp .env.example .env

# Add the gateway key to .env
echo 'AI_GATEWAY_KEY=sk-tASzzTHFKn2uPPfUDLPgqg' >> .env

docker-compose up -d
```

Open http://localhost:3080, register an account (first account becomes admin), select **AI Gateway** as the endpoint, and pick a model.

**Podman note:** LibreChat's `docker-compose.yml` contains `host-gateway` which Podman doesn't support. Comment out the `extra_hosts` block in `docker-compose.yml` — containers reach the host via `host.containers.internal` instead.

## Virtual API Keys

Two tiers are pre-configured, representing the governance model:

| Team           | Key                        | RPM | TPM    |
|----------------|----------------------------|-----|--------|
| trusted-user   | `sk-tASzzTHFKn2uPPfUDLPgqg` | 30  | 50,000 |
| community-user | `sk-NjhQIXb2tzRbzcmjl2KtzA` | 10  | 10,000 |

Example request:

```bash
curl -X POST http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer sk-tASzzTHFKn2uPPfUDLPgqg" \
  -H "Content-Type: application/json" \
  -d '{"model":"gemma4","messages":[{"role":"user","content":"Hello"}],"max_tokens":300}'
```

## Smoke Test

```bash
bash tests/smoke-test.sh
```

Exercises both models through both virtual keys and verifies audit logging contains no prompt/response content.

## Audit Logging

Configured to log request metadata only (timestamp, key, model, token counts, latency). Prompt and response content is **never** logged — enforced via `turn_off_message_logging` and `global_disable_no_log_param` in the gateway config.

Verify with:

```bash
docker-compose exec postgres psql -U litellm -c \
  'SELECT request_id, "startTime", model, api_key, total_tokens FROM "LiteLLM_SpendLogs" ORDER BY "startTime" DESC LIMIT 5;'
```

## Telemetry

All telemetry is disabled:

- **LiteLLM**: `--telemetry=False`
- **MeiliSearch**: `MEILI_NO_ANALYTICS=true`
- **LibreChat GTM**: not configured

No component phones home.

## Project Layout

```
docker-compose.yml                  # Postgres + LiteLLM containers
infrastructure/gateway-config.yaml  # LiteLLM proxy configuration
tests/smoke-test.sh                 # End-to-end smoke test
docs/VISION.md                      # Project vision and non-negotiables
frontend/LibreChat/                 # LibreChat web UI (cloned, gitignored)
  librechat.yaml                    # Custom endpoint config → AI Gateway
  docker-compose.override.yml       # Apple Silicon MongoDB fix + config mount
```

## Known Limitations

- **Local only** — no TLS, no reverse proxy, not exposed to the network.
- **No persistent key storage in config** — virtual keys live in Postgres. If the volume is deleted, keys must be recreated.
- **Thinking models need high max_tokens** — Qwen 3.5 and Gemma 4 use reasoning tokens internally. Requests with low `max_tokens` may return empty content.
- **No fallback routing** — models are independent; no automatic failover between them yet.
- **No authentication beyond virtual keys** — no user identity, no SSO.
- **Spend tracking shows $0** — local Ollama has no cost model. Rate limits (RPM/TPM) are the effective governance lever.
- **Two separate compose stacks** — gateway and LibreChat run independently. Must start both.
