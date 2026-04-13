---
sidebar_position: 2
---

# Virtual API Keys

Virtual keys are the governance layer — they control who can access what, and at what rate.

## Concept

Keys belong to **teams**. Teams define rate limits. Any key belonging to a team inherits its limits.

| Team | Purpose | RPM | TPM |
|------|---------|-----|-----|
| trusted-user | Vetted users, higher limits | 30 | 50,000 |
| community-user | General access, lower limits | 10 | 10,000 |

## Creating Teams

```bash
curl -X POST http://localhost:4000/team/new \
  -H "Authorization: Bearer sk-local-dev-master-key" \
  -H "Content-Type: application/json" \
  -d '{
    "team_alias": "trusted-user",
    "tpm_limit": 50000,
    "rpm_limit": 30
  }'
```

## Generating Keys

```bash
curl -X POST http://localhost:4000/key/generate \
  -H "Authorization: Bearer sk-local-dev-master-key" \
  -H "Content-Type: application/json" \
  -d '{
    "team_id": "<team-id-from-above>",
    "key_alias": "trusted-user-key-1"
  }'
```

The response includes the key value (e.g., `sk-tASzzTHFKn2uPPfUDLPgqg`).

## Using Keys

Keys are passed as Bearer tokens, just like OpenAI:

```bash
curl -X POST http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer sk-tASzzTHFKn2uPPfUDLPgqg" \
  -H "Content-Type: application/json" \
  -d '{"model": "gemma4", "messages": [{"role": "user", "content": "Hello"}]}'
```

## Pre-configured Keys (Local Dev)

For local development, two keys are pre-created:

| Key | Team | RPM | TPM |
|-----|------|-----|-----|
| `sk-tASzzTHFKn2uPPfUDLPgqg` | trusted-user | 30 | 50,000 |
| `sk-NjhQIXb2tzRbzcmjl2KtzA` | community-user | 10 | 10,000 |

:::caution
These keys live in Postgres, not in config files. If you delete the Postgres volume (`docker-compose down -v`), you'll need to recreate them.
:::

## Checking Key Info

```bash
curl http://localhost:4000/key/info \
  -H "Authorization: Bearer sk-local-dev-master-key" \
  -H "Content-Type: application/json" \
  -d '{"key": "sk-tASzzTHFKn2uPPfUDLPgqg"}'
```
