package node

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	pool.Exec(ctx, "DELETE FROM kudos_ledger")
	pool.Exec(ctx, "DELETE FROM tasks")
	pool.Exec(ctx, "DELETE FROM community_nodes")

	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestRegisterNode(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	node, err := store.Register(ctx, RegisterRequest{
		Name:      "test-node",
		PublicKey: "base64pubkey_register",
		Models:    []string{"gemma4", "qwen3.5"},
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if node.Status != "pending" {
		t.Fatalf("expected status 'pending', got %s", node.Status)
	}
	if node.Name != "test-node" {
		t.Fatalf("expected name 'test-node', got %s", node.Name)
	}
	if len(node.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(node.Models))
	}
}

func TestRegisterDuplicateKey(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	store.Register(ctx, RegisterRequest{
		Name:      "node-1",
		PublicKey: "duplicate_key_test",
		Models:    []string{"gemma4"},
	})

	_, err := store.Register(ctx, RegisterRequest{
		Name:      "node-2",
		PublicKey: "duplicate_key_test",
		Models:    []string{"gemma4"},
	})
	if err != ErrDuplicateKey {
		t.Fatalf("expected ErrDuplicateKey, got: %v", err)
	}
}

func TestGetByPublicKey(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	store.Register(ctx, RegisterRequest{
		Name:      "lookup-node",
		PublicKey: "lookup_key_test",
		Models:    []string{"gemma4"},
	})

	node, err := store.GetByPublicKey(ctx, "lookup_key_test")
	if err != nil {
		t.Fatalf("GetByPublicKey failed: %v", err)
	}
	if node.Name != "lookup-node" {
		t.Fatalf("expected 'lookup-node', got %s", node.Name)
	}
}

func TestGetByPublicKeyNotFound(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	_, err := store.GetByPublicKey(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestApproveAndActivate(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	node, _ := store.Register(ctx, RegisterRequest{
		Name:      "lifecycle-node",
		PublicKey: "lifecycle_key",
		Models:    []string{"gemma4"},
	})

	// pending → approved
	err := store.Approve(ctx, node.ID)
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	updated, _ := store.GetByPublicKey(ctx, "lifecycle_key")
	if updated.Status != "approved" {
		t.Fatalf("expected 'approved', got %s", updated.Status)
	}

	// approved → active
	err = store.Activate(ctx, node.ID)
	if err != nil {
		t.Fatalf("Activate failed: %v", err)
	}

	updated, _ = store.GetByPublicKey(ctx, "lifecycle_key")
	if updated.Status != "active" {
		t.Fatalf("expected 'active', got %s", updated.Status)
	}
	if updated.LastHeartbeat == nil {
		t.Fatal("expected last_heartbeat to be set")
	}
}

func TestHeartbeat(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	node, _ := store.Register(ctx, RegisterRequest{
		Name:      "heartbeat-node",
		PublicKey: "heartbeat_key",
		Models:    []string{"gemma4"},
	})

	// Heartbeat on pending node should fail
	err := store.Heartbeat(ctx, node.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for pending node, got: %v", err)
	}

	store.Approve(ctx, node.ID)
	store.Activate(ctx, node.ID)

	// Heartbeat on active node should work
	err = store.Heartbeat(ctx, node.ID)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
}

func TestApproveNonPending(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	node, _ := store.Register(ctx, RegisterRequest{
		Name:      "double-approve",
		PublicKey: "double_approve_key",
		Models:    []string{"gemma4"},
	})

	store.Approve(ctx, node.ID)

	// Approving again should fail
	err := store.Approve(ctx, node.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for already-approved node, got: %v", err)
	}
}
