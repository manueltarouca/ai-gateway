package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoTask   = errors.New("no task available")
	ErrNotFound = errors.New("task not found")
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Task struct {
	ID          string    `json:"id"`
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Parameters  json.RawMessage `json:"parameters"`
	Status      string    `json:"status"`
	AssignedNode *string  `json:"assigned_node,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	AssignedAt  *time.Time `json:"assigned_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type TaskResult struct {
	Content    string `json:"content"`
	TokensUsed int    `json:"tokens_used"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Enqueue creates a new task and returns its ID.
func (s *Store) Enqueue(ctx context.Context, model string, messages []Message, maxTokens int) (string, error) {
	msgJSON, err := json.Marshal(messages)
	if err != nil {
		return "", err
	}

	var id string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO tasks (model, messages, max_tokens, status)
		 VALUES ($1, $2, $3, 'queued')
		 RETURNING id`,
		model, msgJSON, maxTokens,
	).Scan(&id)
	return id, err
}

// Poll atomically claims the next queued task for the given models.
// Returns ErrNoTask if the queue is empty.
func (s *Store) Poll(ctx context.Context, nodeID string, models []string) (*Task, error) {
	var t Task
	var msgRaw []byte

	err := s.pool.QueryRow(ctx,
		`UPDATE tasks
		 SET status = 'assigned', assigned_node = $1, assigned_at = now()
		 WHERE id = (
		   SELECT id FROM tasks
		   WHERE status = 'queued' AND model = ANY($2)
		   ORDER BY created_at ASC
		   FOR UPDATE SKIP LOCKED
		   LIMIT 1
		 )
		 RETURNING id, model, messages, max_tokens, parameters, status, assigned_node, created_at, assigned_at`,
		nodeID, models,
	).Scan(&t.ID, &t.Model, &msgRaw, &t.MaxTokens, &t.Parameters, &t.Status, &t.AssignedNode, &t.CreatedAt, &t.AssignedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoTask
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(msgRaw, &t.Messages); err != nil {
		return nil, err
	}
	return &t, nil
}

// Complete marks a task as done and records the result.
func (s *Store) Complete(ctx context.Context, taskID, nodeID string, result TaskResult) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE tasks
		 SET status = 'done', result = $1, tokens_used = $2, completed_at = now()
		 WHERE id = $3 AND assigned_node = $4 AND status = 'assigned'`,
		resultJSON, result.TokensUsed, taskID, nodeID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Fail marks a task as failed and returns it to the queue.
func (s *Store) Fail(ctx context.Context, taskID, nodeID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tasks
		 SET status = 'queued', assigned_node = NULL, assigned_at = NULL
		 WHERE id = $1 AND assigned_node = $2 AND status = 'assigned'`,
		taskID, nodeID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Get retrieves a task by ID.
func (s *Store) Get(ctx context.Context, taskID string) (*Task, error) {
	var t Task
	var msgRaw []byte

	err := s.pool.QueryRow(ctx,
		`SELECT id, model, messages, max_tokens, parameters, status, assigned_node, created_at, assigned_at, completed_at
		 FROM tasks WHERE id = $1`,
		taskID,
	).Scan(&t.ID, &t.Model, &msgRaw, &t.MaxTokens, &t.Parameters, &t.Status, &t.AssignedNode, &t.CreatedAt, &t.AssignedAt, &t.CompletedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(msgRaw, &t.Messages); err != nil {
		return nil, err
	}
	return &t, nil
}

// WaitForResult blocks until the task is completed or the context is cancelled.
func (s *Store) WaitForResult(ctx context.Context, taskID string) (*Task, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		task, err := s.Get(ctx, taskID)
		if err != nil {
			return nil, err
		}
		if task.Status == "done" || task.Status == "failed" {
			return task, nil
		}

		// Poll interval — will be replaced with LISTEN/NOTIFY later
		time.Sleep(200 * time.Millisecond)
	}
}
