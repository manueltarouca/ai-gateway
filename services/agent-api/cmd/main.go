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
	nodes *node.Store
	tasks *queue.Store
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

	s := &server{
		nodes: node.NewStore(pool),
		tasks: queue.NewStore(pool),
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
