-- Community tier tables for agent-based distributed inference
-- Runs in the same Postgres instance as LiteLLM

CREATE TABLE IF NOT EXISTS community_nodes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    public_key      TEXT NOT NULL UNIQUE,  -- Ed25519 public key, base64-encoded
    models          JSONB NOT NULL DEFAULT '[]',
    vram_mb         INTEGER,
    status          TEXT NOT NULL DEFAULT 'pending',  -- pending, approved, active, suspended
    last_heartbeat  TIMESTAMPTZ,
    kudos           BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tasks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model           TEXT NOT NULL,
    messages        JSONB NOT NULL,
    max_tokens      INTEGER NOT NULL DEFAULT 300,
    parameters      JSONB NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'queued',  -- queued, assigned, running, done, failed
    assigned_node   UUID REFERENCES community_nodes(id),
    result          JSONB,
    tokens_used     INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    assigned_at     TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS kudos_ledger (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id         UUID NOT NULL REFERENCES community_nodes(id),
    task_id         UUID REFERENCES tasks(id),
    amount          BIGINT NOT NULL,
    reason          TEXT NOT NULL,  -- 'task_completed', 'bonus', 'penalty'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tasks_status_model ON tasks(status, model);
CREATE INDEX IF NOT EXISTS idx_tasks_assigned_node ON tasks(assigned_node);
CREATE INDEX IF NOT EXISTS idx_kudos_node ON kudos_ledger(node_id);
CREATE INDEX IF NOT EXISTS idx_nodes_status ON community_nodes(status);
