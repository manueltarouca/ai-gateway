---
sidebar_position: 1
---

# Community Tier Overview

The community tier enables volunteers to contribute compute to the gateway by running the `gateway-agent` on their own hardware alongside Ollama.

## How It Works

```
┌─────────────────────────────────────────────────┐
│              Volunteer's Machine                 │
│                                                  │
│  ┌──────────────┐      ┌──────────────────────┐ │
│  │   Ollama      │◄────│   gateway-agent       │ │
│  │   (local)     │────►│                       │ │
│  │               │     │ • polls for tasks      │ │
│  │  gemma4       │     │ • runs inference local │ │
│  │  qwen3.5      │     │ • pushes results back  │ │
│  └──────────────┘      │ • Ed25519 signed       │ │
│                        └───────────┬────────────┘ │
└────────────────────────────────────┼──────────────┘
                                     │ HTTPS (outbound only)
                                     ▼
                            ┌─────────────────┐
                            │   agent-api      │
                            │   (central)      │
                            │                  │
                            │  task queue      │
                            │  node registry   │
                            │  kudos ledger    │
                            └─────────────────┘
```

## Key Properties

- **Outbound-only networking** — the agent polls the gateway. No inbound ports needed. Works behind NAT, firewalls, home routers.
- **Ollama is the only dependency** — if you can run Ollama, you can contribute compute.
- **The gateway never touches the volunteer's Ollama** — it only knows the agent. The agent is the trust boundary.
- **Ed25519 authentication** — every request from the agent is signed with a private key. No shared secrets.
- **Admin approval required** — nodes must be approved before they can accept tasks.

## Node Lifecycle

```
Register → Pending → [Admin Approves] → Approved → [First Heartbeat] → Active
                                                                          ↓
                                                        [Admin Suspends] → Suspended
```

## Integration with LiteLLM

The community tier is registered as a LiteLLM custom provider. When a user requests a model, LiteLLM tries the trusted tier first (`order: 1`). If it fails, it falls back to the community tier (`order: 2`), which enqueues a task and waits for an agent to complete it.

From the user's perspective, the API is identical. They don't know or need to know which tier served their request.

## Task Flow

1. User sends request to LiteLLM
2. LiteLLM's CommunityProvider enqueues a task in Postgres
3. Agent polls `GET /api/tasks/next`, receives the task
4. Agent runs inference against local Ollama
5. Agent submits result via `POST /api/tasks/{id}/result`
6. CommunityProvider receives the result, returns it to LiteLLM
7. LiteLLM returns the response to the user
