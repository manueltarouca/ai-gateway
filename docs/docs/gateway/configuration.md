---
sidebar_position: 1
---

# Gateway Configuration

The gateway is configured via `infrastructure/gateway-config.yaml`, mounted into the LiteLLM container.

## Model List

Each entry defines a backend the gateway can route to:

```yaml
model_list:
  - model_name: gemma4           # name users request
    litellm_params:
      model: ollama/gemma4:e4b   # provider/model identifier
      api_base: http://host.containers.internal:11434
      order: 1                   # routing priority
```

### Thinking Models

Models like Qwen 3.5 and Gemma 4 use internal reasoning tokens. Add this to merge thinking content into the response:

```yaml
      merge_reasoning_content_in_choices: true
```

Without this, responses may appear empty because reasoning tokens consume the `max_tokens` budget.

## Privacy Settings

```yaml
litellm_settings:
  turn_off_message_logging: true       # no prompts/responses in logs
  global_disable_no_log_param: true    # clients can't bypass logging
```

These are non-negotiable. The first prevents content from being stored in the spend logs table. The second prevents clients from using the `no-log` parameter to skip audit logging entirely.

## Telemetry

```yaml
command:
  - '--telemetry=False'
```

Disabled at the CLI level. No phone-home from any component.

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `DATABASE_URL` | Postgres connection string |
| `LITELLM_MASTER_KEY` | Admin key for LiteLLM and API management |
| `AGENT_API_URL` | URL of the agent-api service (used by CommunityProvider) |

## Full Example

```yaml
model_list:
  # Trusted tier
  - model_name: qwen3.5
    litellm_params:
      model: ollama/qwen3.5:9B
      api_base: http://host.containers.internal:11434
      merge_reasoning_content_in_choices: true
      order: 1
  - model_name: gemma4
    litellm_params:
      model: ollama/gemma4:e4b
      api_base: http://host.containers.internal:11434
      merge_reasoning_content_in_choices: true
      order: 1

  # Community tier
  - model_name: qwen3.5
    litellm_params:
      model: community/qwen3.5:9B
      order: 2
  - model_name: gemma4
    litellm_params:
      model: community/gemma4:e4b
      order: 2

litellm_settings:
  turn_off_message_logging: true
  global_disable_no_log_param: true
  custom_provider_map:
    - provider: community
      custom_handler: community.provider.handler

general_settings:
  master_key: sk-local-dev-master-key
  database_url: postgresql://litellm:litellm-local-dev@postgres:5432/litellm
```
