---
sidebar_position: 3
---

# Audit Logging

Every request through the gateway is logged for quota accounting and abuse handling. **Prompt and response content is never logged.**

## What Gets Logged

| Field | Logged | Example |
|-------|--------|---------|
| Request ID | Yes | `chatcmpl-03666351-...` |
| Timestamp | Yes | `2026-04-09 19:49:23` |
| Model | Yes | `ollama/qwen3.5:9B` |
| API key (hashed) | Yes | `9936e916...` |
| Team ID | Yes | `1f9c4fa7-...` |
| Token counts | Yes | prompt: 21, completion: 181 |
| Latency | Yes | 7.0s |
| Prompt content | **Never** | — |
| Response content | **Never** | — |

## How It's Enforced

Two settings in `gateway-config.yaml`:

```yaml
litellm_settings:
  turn_off_message_logging: true
  global_disable_no_log_param: true
```

- `turn_off_message_logging` — prevents LiteLLM from storing message content in the `messages` and `response` columns of the spend logs table. They remain empty (`{}`).
- `global_disable_no_log_param` — prevents clients from passing `no-log: true` to skip audit logging entirely. Every request is logged.

## Querying Logs

```bash
docker-compose exec postgres psql -U litellm -c '
  SELECT request_id, "startTime", model, api_key, total_tokens
  FROM "LiteLLM_SpendLogs"
  ORDER BY "startTime" DESC
  LIMIT 10;
'
```

## Verifying Content Is Not Logged

```bash
docker-compose exec postgres psql -U litellm -c '
  SELECT messages, response
  FROM "LiteLLM_SpendLogs"
  ORDER BY "startTime" DESC
  LIMIT 1;
'
```

Both columns should show `{}`.
