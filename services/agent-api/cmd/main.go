package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/manueltarouca/ai-gateway/services/agent-api/internal/auth"
	"github.com/manueltarouca/ai-gateway/services/agent-api/internal/node"
	"github.com/manueltarouca/ai-gateway/services/agent-api/internal/queue"
)

type server struct {
	nodes    *node.Store
	tasks    *queue.Store
	adminKey string
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	adminKey := os.Getenv("ADMIN_KEY")
	if adminKey == "" {
		adminKey = os.Getenv("LITELLM_MASTER_KEY")
	}
	if adminKey == "" {
		log.Fatal("ADMIN_KEY or LITELLM_MASTER_KEY is required")
	}

	s := &server{
		nodes:    node.NewStore(pool),
		tasks:    queue.NewStore(pool),
		adminKey: adminKey,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/nodes/register", s.handleRegister)
	mux.HandleFunc("POST /api/nodes/heartbeat", s.withAuth(s.handleHeartbeat))
	mux.HandleFunc("GET /api/tasks/next", s.withAuth(s.handlePollTask))
	mux.HandleFunc("POST /api/tasks/{id}/result", s.withAuth(s.handleCompleteTask))
	mux.HandleFunc("POST /api/tasks/{id}/fail", s.withAuth(s.handleFailTask))
	mux.HandleFunc("POST /api/tasks", s.handleEnqueue)
	mux.HandleFunc("GET /api/tasks/{id}", s.handleGetTask)
	mux.HandleFunc("GET /health", s.handleHealth)

	// Admin endpoints — protected by master key
	mux.HandleFunc("GET /api/admin/nodes", s.withAdmin(s.handleAdminListNodes))
	mux.HandleFunc("POST /api/admin/nodes/{id}/approve", s.withAdmin(s.handleAdminApproveNode))
	mux.HandleFunc("POST /api/admin/nodes/{id}/suspend", s.withAdmin(s.handleAdminSuspendNode))

	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}

	log.Printf("agent-api listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// withAuth wraps a handler with Ed25519 signature verification.
func (s *server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubKey := r.Header.Get("X-Node-PublicKey")
		timestamp := r.Header.Get("X-Timestamp")
		signature := r.Header.Get("X-Signature")

		if pubKey == "" || timestamp == "" || signature == "" {
			writeError(w, http.StatusUnauthorized, "missing auth headers")
			return
		}

		if err := auth.VerifyRequest(pubKey, r.Method, r.URL.Path, timestamp, signature); err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}

		// Look up node and verify it's active
		n, err := s.nodes.GetByPublicKey(r.Context(), pubKey)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unknown node")
			return
		}
		if n.Status != "active" && n.Status != "approved" {
			writeError(w, http.StatusForbidden, "node not active")
			return
		}

		// Inject node ID into context
		ctx := context.WithValue(r.Context(), contextKeyNodeID, n.ID)
		ctx = context.WithValue(ctx, contextKeyNodeModels, n.Models)
		next(w, r.WithContext(ctx))
	}
}

type contextKey string

const (
	contextKeyNodeID     contextKey = "nodeID"
	contextKeyNodeModels contextKey = "nodeModels"
)

func (s *server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req node.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.PublicKey == "" || len(req.Models) == 0 {
		writeError(w, http.StatusBadRequest, "name, public_key, and models are required")
		return
	}

	n, err := s.nodes.Register(r.Context(), req)
	if errors.Is(err, node.ErrDuplicateKey) {
		writeError(w, http.StatusConflict, "public key already registered")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "registration failed")
		return
	}

	writeJSON(w, http.StatusCreated, n)
}

func (s *server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	nodeID := r.Context().Value(contextKeyNodeID).(string)

	// First heartbeat activates an approved node
	n, _ := s.nodes.GetByPublicKey(r.Context(), r.Header.Get("X-Node-PublicKey"))
	if n != nil && n.Status == "approved" {
		s.nodes.Activate(r.Context(), nodeID)
	}

	if err := s.nodes.Heartbeat(r.Context(), nodeID); err != nil {
		writeError(w, http.StatusInternalServerError, "heartbeat failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handlePollTask(w http.ResponseWriter, r *http.Request) {
	nodeID := r.Context().Value(contextKeyNodeID).(string)
	models := r.Context().Value(contextKeyNodeModels).([]string)

	// Allow filtering by model query param
	if m := r.URL.Query().Get("models"); m != "" {
		models = strings.Split(m, ",")
	}

	task, err := s.tasks.Poll(r.Context(), nodeID, models)
	if errors.Is(err, queue.ErrNoTask) {
		writeJSON(w, http.StatusNoContent, nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "poll failed")
		return
	}

	writeJSON(w, http.StatusOK, task)
}

func (s *server) handleCompleteTask(w http.ResponseWriter, r *http.Request) {
	nodeID := r.Context().Value(contextKeyNodeID).(string)
	taskID := r.PathValue("id")

	var result queue.TaskResult
	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := s.tasks.Complete(r.Context(), taskID, nodeID, result)
	if errors.Is(err, queue.ErrNotFound) {
		writeError(w, http.StatusNotFound, "task not found or not assigned to you")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "complete failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleFailTask(w http.ResponseWriter, r *http.Request) {
	nodeID := r.Context().Value(contextKeyNodeID).(string)
	taskID := r.PathValue("id")

	err := s.tasks.Fail(r.Context(), taskID, nodeID)
	if errors.Is(err, queue.ErrNotFound) {
		writeError(w, http.StatusNotFound, "task not found or not assigned to you")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "fail failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "requeued"})
}

func (s *server) handleEnqueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string          `json:"model"`
		Messages []queue.Message `json:"messages"`
		MaxTokens int           `json:"max_tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MaxTokens == 0 {
		req.MaxTokens = 300
	}

	id, err := s.tasks.Enqueue(r.Context(), req.Model, req.Messages, req.MaxTokens)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "enqueue failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"task_id": id})
}

func (s *server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")

	task, err := s.tasks.Get(r.Context(), taskID)
	if errors.Is(err, queue.ErrNotFound) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get failed")
		return
	}

	writeJSON(w, http.StatusOK, task)
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// withAdmin checks for the master key in the Authorization header.
func (s *server) withAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if key != s.adminKey {
			writeError(w, http.StatusUnauthorized, "invalid admin key")
			return
		}
		next(w, r)
	}
}

func (s *server) handleAdminListNodes(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	nodes, err := s.nodes.List(r.Context(), status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	if nodes == nil {
		nodes = []node.Node{}
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *server) handleAdminApproveNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	err := s.nodes.Approve(r.Context(), nodeID)
	if errors.Is(err, node.ErrNotFound) {
		writeError(w, http.StatusNotFound, "node not found or not in pending status")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "approve failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (s *server) handleAdminSuspendNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	err := s.nodes.Suspend(r.Context(), nodeID)
	if errors.Is(err, node.ErrNotFound) {
		writeError(w, http.StatusNotFound, "node not found or already suspended")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "suspend failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "suspended"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
