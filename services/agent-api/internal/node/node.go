package node

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound       = errors.New("node not found")
	ErrDuplicateKey   = errors.New("public key already registered")
	ErrNotApproved    = errors.New("node not approved")
)

type Node struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	PublicKey     string    `json:"public_key"`
	Models        []string  `json:"models"`
	VramMB        *int      `json:"vram_mb,omitempty"`
	Status        string    `json:"status"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	Kudos         int64     `json:"kudos"`
	CreatedAt     time.Time `json:"created_at"`
}

type RegisterRequest struct {
	Name      string   `json:"name"`
	PublicKey string   `json:"public_key"`
	Models    []string `json:"models"`
	VramMB    *int     `json:"vram_mb,omitempty"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Register creates a new community node in pending status.
func (s *Store) Register(ctx context.Context, req RegisterRequest) (*Node, error) {
	modelsJSON, err := json.Marshal(req.Models)
	if err != nil {
		return nil, err
	}

	var n Node
	var modelsRaw []byte
	err = s.pool.QueryRow(ctx,
		`INSERT INTO community_nodes (name, public_key, models, vram_mb, status)
		 VALUES ($1, $2, $3, $4, 'pending')
		 RETURNING id, name, public_key, models, vram_mb, status, kudos, created_at`,
		req.Name, req.PublicKey, modelsJSON, req.VramMB,
	).Scan(&n.ID, &n.Name, &n.PublicKey, &modelsRaw, &n.VramMB, &n.Status, &n.Kudos, &n.CreatedAt)

	if err != nil {
		if isDuplicateKey(err) {
			return nil, ErrDuplicateKey
		}
		return nil, err
	}

	json.Unmarshal(modelsRaw, &n.Models)
	return &n, nil
}

// GetByPublicKey looks up a node by its Ed25519 public key.
func (s *Store) GetByPublicKey(ctx context.Context, publicKey string) (*Node, error) {
	var n Node
	var modelsRaw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, public_key, models, vram_mb, status, last_heartbeat, kudos, created_at
		 FROM community_nodes WHERE public_key = $1`,
		publicKey,
	).Scan(&n.ID, &n.Name, &n.PublicKey, &modelsRaw, &n.VramMB, &n.Status, &n.LastHeartbeat, &n.Kudos, &n.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(modelsRaw, &n.Models)
	return &n, nil
}

// Heartbeat updates the last heartbeat timestamp for an active node.
func (s *Store) Heartbeat(ctx context.Context, nodeID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE community_nodes SET last_heartbeat = now(), updated_at = now()
		 WHERE id = $1 AND status IN ('active', 'approved')`,
		nodeID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Approve transitions a node from pending to approved.
func (s *Store) Approve(ctx context.Context, nodeID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE community_nodes SET status = 'approved', updated_at = now()
		 WHERE id = $1 AND status = 'pending'`,
		nodeID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Activate transitions a node from approved to active (first heartbeat).
func (s *Store) Activate(ctx context.Context, nodeID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE community_nodes SET status = 'active', last_heartbeat = now(), updated_at = now()
		 WHERE id = $1 AND status = 'approved'`,
		nodeID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns all community nodes, optionally filtered by status.
func (s *Store) List(ctx context.Context, status string) ([]Node, error) {
	var query string
	var args []any

	if status != "" {
		query = `SELECT id, name, public_key, models, vram_mb, status, last_heartbeat, kudos, created_at
			FROM community_nodes WHERE status = $1 ORDER BY created_at DESC`
		args = append(args, status)
	} else {
		query = `SELECT id, name, public_key, models, vram_mb, status, last_heartbeat, kudos, created_at
			FROM community_nodes ORDER BY created_at DESC`
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		var modelsRaw []byte
		if err := rows.Scan(&n.ID, &n.Name, &n.PublicKey, &modelsRaw, &n.VramMB, &n.Status, &n.LastHeartbeat, &n.Kudos, &n.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(modelsRaw, &n.Models)
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// Suspend transitions any node to suspended status.
func (s *Store) Suspend(ctx context.Context, nodeID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE community_nodes SET status = 'suspended', updated_at = now()
		 WHERE id = $1 AND status != 'suspended'`,
		nodeID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func isDuplicateKey(err error) bool {
	return err != nil && containsDuplicate(err.Error())
}

func containsDuplicate(s string) bool {
	return len(s) > 0 && (contains(s, "duplicate key") || contains(s, "unique constraint"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
