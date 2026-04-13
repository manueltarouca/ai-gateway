package queue

import (
	"context"
	"os"
	"testing"
	"time"

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

	// Clean slate
	pool.Exec(ctx, "DELETE FROM kudos_ledger")
	pool.Exec(ctx, "DELETE FROM tasks")
	pool.Exec(ctx, "DELETE FROM community_nodes")

	// Insert a test node
	pool.Exec(ctx, `INSERT INTO community_nodes (id, name, public_key, models, status)
		VALUES ('00000000-0000-0000-0000-000000000001', 'test-node', 'testkey', '["gemma4"]', 'active')
		ON CONFLICT (id) DO NOTHING`)

	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestEnqueueAndPoll(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	// Enqueue a task
	taskID, err := store.Enqueue(ctx, "gemma4", []Message{
		{Role: "user", Content: "Hello"},
	}, 100)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if taskID == "" {
		t.Fatal("expected non-empty task ID")
	}

	// Poll for it
	task, err := store.Poll(ctx, "00000000-0000-0000-0000-000000000001", []string{"gemma4"})
	if err != nil {
		t.Fatalf("Poll failed: %v", err)
	}
	if task.ID != taskID {
		t.Fatalf("expected task %s, got %s", taskID, task.ID)
	}
	if task.Status != "assigned" {
		t.Fatalf("expected status 'assigned', got %s", task.Status)
	}
	if task.Messages[0].Content != "Hello" {
		t.Fatalf("expected message 'Hello', got %s", task.Messages[0].Content)
	}
}

func TestPollEmptyQueue(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	_, err := store.Poll(ctx, "00000000-0000-0000-0000-000000000001", []string{"gemma4"})
	if err != ErrNoTask {
		t.Fatalf("expected ErrNoTask, got: %v", err)
	}
}

func TestPollOnlyMatchingModels(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	// Enqueue for gemma4
	store.Enqueue(ctx, "gemma4", []Message{{Role: "user", Content: "test"}}, 100)

	// Poll for qwen — should find nothing
	_, err := store.Poll(ctx, "00000000-0000-0000-0000-000000000001", []string{"qwen3.5"})
	if err != ErrNoTask {
		t.Fatalf("expected ErrNoTask for non-matching model, got: %v", err)
	}

	// Poll for gemma4 — should find it
	task, err := store.Poll(ctx, "00000000-0000-0000-0000-000000000001", []string{"gemma4"})
	if err != nil {
		t.Fatalf("Poll for matching model failed: %v", err)
	}
	if task.Model != "gemma4" {
		t.Fatalf("expected model gemma4, got %s", task.Model)
	}
}

func TestCompleteTask(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	taskID, _ := store.Enqueue(ctx, "gemma4", []Message{{Role: "user", Content: "test"}}, 100)
	store.Poll(ctx, "00000000-0000-0000-0000-000000000001", []string{"gemma4"})

	err := store.Complete(ctx, taskID, "00000000-0000-0000-0000-000000000001", TaskResult{
		Content:    "Hello back",
		TokensUsed: 42,
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	task, err := store.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if task.Status != "done" {
		t.Fatalf("expected status 'done', got %s", task.Status)
	}
	if task.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
}

func TestCompleteRejectsWrongNode(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	taskID, _ := store.Enqueue(ctx, "gemma4", []Message{{Role: "user", Content: "test"}}, 100)
	store.Poll(ctx, "00000000-0000-0000-0000-000000000001", []string{"gemma4"})

	err := store.Complete(ctx, taskID, "00000000-0000-0000-0000-000000000099", TaskResult{Content: "hack", TokensUsed: 1})
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for wrong node, got: %v", err)
	}
}

func TestFailRequeuesTask(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	taskID, _ := store.Enqueue(ctx, "gemma4", []Message{{Role: "user", Content: "test"}}, 100)
	store.Poll(ctx, "00000000-0000-0000-0000-000000000001", []string{"gemma4"})

	err := store.Fail(ctx, taskID, "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	// Should be pollable again
	task, err := store.Poll(ctx, "00000000-0000-0000-0000-000000000001", []string{"gemma4"})
	if err != nil {
		t.Fatalf("second Poll failed: %v", err)
	}
	if task.ID != taskID {
		t.Fatalf("expected requeued task %s, got %s", taskID, task.ID)
	}
}

func TestWaitForResult(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	taskID, _ := store.Enqueue(ctx, "gemma4", []Message{{Role: "user", Content: "test"}}, 100)
	store.Poll(ctx, "00000000-0000-0000-0000-000000000001", []string{"gemma4"})

	// Complete in background
	go func() {
		time.Sleep(500 * time.Millisecond)
		store.Complete(context.Background(), taskID, "00000000-0000-0000-0000-000000000001", TaskResult{
			Content:    "result",
			TokensUsed: 10,
		})
	}()

	task, err := store.WaitForResult(ctx, taskID)
	if err != nil {
		t.Fatalf("WaitForResult failed: %v", err)
	}
	if task.Status != "done" {
		t.Fatalf("expected done, got %s", task.Status)
	}
}

func TestPollFIFOOrder(t *testing.T) {
	pool := setupTestDB(t)
	store := NewStore(pool)
	ctx := context.Background()

	id1, _ := store.Enqueue(ctx, "gemma4", []Message{{Role: "user", Content: "first"}}, 100)
	time.Sleep(10 * time.Millisecond)
	store.Enqueue(ctx, "gemma4", []Message{{Role: "user", Content: "second"}}, 100)

	task, _ := store.Poll(ctx, "00000000-0000-0000-0000-000000000001", []string{"gemma4"})
	if task.ID != id1 {
		t.Fatalf("expected first task, got %s", task.ID)
	}
}
