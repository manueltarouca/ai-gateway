#!/usr/bin/env bash
set -uo pipefail

GATEWAY_URL="http://localhost:4000"
TRUSTED_KEY="sk-tASzzTHFKn2uPPfUDLPgqg"
COMMUNITY_KEY="sk-NjhQIXb2tzRbzcmjl2KtzA"
MODELS=("qwen3.5" "gemma4")

PASS=0
FAIL=0

check() {
  local description="$1"
  local result="$2"
  if [ "$result" = "true" ]; then
    echo "  PASS: $description"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $description"
    FAIL=$((FAIL + 1))
  fi
}

call_model() {
  local key="$1"
  local model="$2"
  local response
  response=$(curl -s --max-time 90 -X POST "$GATEWAY_URL/v1/chat/completions" \
    -H "Authorization: Bearer $key" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$model\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply with exactly: ok\"}],\"max_tokens\":300}")
  local content
  content=$(echo "$response" | python3 -c "import sys,json; print(json.load(sys.stdin)['choices'][0]['message']['content'])" 2>/dev/null) || content=""
  if [ -n "$content" ]; then
    check "$model responds with content" "true"
  else
    echo "    Response was: $response"
    check "$model responds with content" "false"
  fi
}

echo "=== AI Gateway Smoke Test ==="
echo ""

# 1. Health check
echo "[1/4] Health check"
health=$(curl -s -H "Authorization: Bearer $TRUSTED_KEY" "$GATEWAY_URL/health")
healthy_count=$(echo "$health" | python3 -c "import sys,json; print(json.load(sys.stdin)['healthy_count'])" 2>/dev/null) || healthy_count=0
if [ "$healthy_count" -ge 2 ]; then
  check "Gateway is healthy with $healthy_count backends" "true"
else
  check "Gateway is healthy with $healthy_count backends" "false"
fi
echo ""

# 2. Both models respond via trusted-user key
echo "[2/4] Trusted-user key — both models"
for model in "${MODELS[@]}"; do
  call_model "$TRUSTED_KEY" "$model"
done
echo ""

# 3. Both models respond via community-user key
echo "[3/4] Community-user key — both models"
for model in "${MODELS[@]}"; do
  call_model "$COMMUNITY_KEY" "$model"
done
echo ""

# 4. Audit logging — verify no prompt/response content stored
echo "[4/4] Audit logging — no prompt/response content in database"
latest_log=$(docker-compose exec -T postgres psql -U litellm -t -A -c \
  'SELECT messages, response FROM "LiteLLM_SpendLogs" ORDER BY "startTime" DESC LIMIT 1;' 2>/dev/null) || latest_log=""
messages_empty=$(echo "$latest_log" | python3 -c "
import sys
line = sys.stdin.read().strip()
parts = line.split('|')
print('true' if len(parts) >= 2 and parts[0].strip() in ('{}', '') and parts[1].strip() in ('{}', '') else 'false')
" 2>/dev/null) || messages_empty="false"
check "Spend logs contain no prompt/response content" "$messages_empty"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
exit "$FAIL"
