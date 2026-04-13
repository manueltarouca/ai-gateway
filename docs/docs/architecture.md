---
sidebar_position: 2
---

# Architecture

## Overview

```
┌─────────────────────────────────────────────────────────────┐
│ Clients (curl, SDKs, LibreChat)                             │
└─────────────┬───────────────────────────────────────────────┘
              │ OpenAI-compatible API
              ▼
┌─────────────────────────────────────────────────────────────┐
│ LiteLLM Gateway (port 4000)                                 │
│  • Virtual API keys      • Rate limits (RPM/TPM)            │
│  • Audit logging         • Two-tier routing                 │
│  • No telemetry          • No prompt/response logging       │
└──────┬──────────────────────────────┬───────────────────────┘
       │ order: 1 (preferred)         │ order: 2 (fallback)
       ▼                              ▼
┌──────────────┐        ┌───────────────────────────────────┐
│ Trusted Tier │        │ Community Tier                     │
│              │        │                                    │
│ Direct to    │        │ CommunityProvider → agent-api      │
│ local Ollama │        │         ↕ task queue (Postgres)    │
│              │        │ gateway-agent polls, infers, returns│
└──────────────┘        └───────────────────────────────────┘
```

## Design Principles

These come directly from the project's founding document:

- **The gateway is the product, the compute is pluggable.** Identity, quota, audit, routing, and privacy enforcement live at the gateway layer. Model backends are interchangeable.
- **Two tiers of compute.** Trusted tier is hardware we control directly. Community tier is contributed by volunteers under an async task queue.
- **No data extraction.** Prompts and responses are never logged. Audit logs contain only request metadata.
- **Open weights only.** We do not proxy to closed API providers.
- **Boring tech wins.** LiteLLM, Postgres, Go, Ollama — proven, well-documented tools.

## Components

| Component | Language | Purpose |
|-----------|----------|---------|
| **LiteLLM** | Python | OpenAI-compatible gateway, routing, virtual keys, audit |
| **agent-api** | Go | Task queue API, node registration, admin endpoints |
| **gateway-agent** | Go | Community node binary — polls tasks, runs inference |
| **CommunityProvider** | Python | LiteLLM custom provider bridging gateway to task queue |
| **Postgres** | — | Shared state: LiteLLM tables, tasks, nodes, kudos |

## Routing

LiteLLM's `order` parameter controls tier preference:

```yaml
model_list:
  # Try trusted first
  - model_name: gemma4
    litellm_params:
      model: ollama/gemma4:e4b
      order: 1

  # Fall back to community
  - model_name: gemma4
    litellm_params:
      model: community/gemma4:e4b
      order: 2
```

From the user's perspective, they request `gemma4` — the gateway decides which tier handles it. If the trusted backend is down or overloaded, the community tier picks up the request transparently.

## Security Model

### Gateway Layer
- Virtual API keys with per-team rate limits
- Master key for admin operations
- No prompt/response content in logs (enforced in config)
- All telemetry disabled

### Community Tier
- Ed25519 key pairs per node — generated on first run
- All agent requests are signed (method + path + timestamp)
- Nodes require admin approval before accepting tasks
- Nodes can be suspended instantly via admin API
- Agents never access the database directly
