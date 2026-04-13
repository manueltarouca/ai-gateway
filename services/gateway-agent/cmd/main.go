package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/manueltarouca/ai-gateway/services/gateway-agent/internal/auth"
	"github.com/manueltarouca/ai-gateway/services/gateway-agent/internal/inference"
)

type config struct {
	gatewayURL   string
	ollamaURL    string
	nodeName     string
	models       []string
	keyDir       string
	pollInterval time.Duration
}

func main() {
	var cfg config
	var modelsFlag string

	flag.StringVar(&cfg.gatewayURL, "gateway", "http://localhost:9090", "Agent API URL")
	flag.StringVar(&cfg.ollamaURL, "ollama", "http://localhost:11434", "Ollama API URL")
	flag.StringVar(&cfg.nodeName, "name", "", "Node name")
	flag.StringVar(&modelsFlag, "models", "", "Comma-separated model names to serve")
	flag.StringVar(&cfg.keyDir, "keys", "", "Directory for Ed25519 keys (default: ~/.gateway-agent)")
	flag.DurationVar(&cfg.pollInterval, "poll", 2*time.Second, "Poll interval")
	flag.Parse()

	if cfg.keyDir == "" {
		home, _ := os.UserHomeDir()
		cfg.keyDir = filepath.Join(home, ".gateway-agent")
	}

	if cfg.nodeName == "" {
		hostname, _ := os.Hostname()
		cfg.nodeName = hostname
	}

	// Load or generate Ed25519 key pair
	kp, err := auth.LoadOrGenerate(cfg.keyDir)
	if err != nil {
		log.Fatalf("key setup failed: %v", err)
	}
	log.Printf("public key: %s", kp.PublicKey)

	// Discover models from Ollama if not specified
	ollama := inference.NewClient(cfg.ollamaURL)
	if modelsFlag == "" {
		models, err := ollama.ListModels(context.Background())
		if err != nil {
			log.Fatalf("failed to list Ollama models: %v", err)
		}
		cfg.models = models
		log.Printf("auto-detected models: %v", cfg.models)
	} else {
		cfg.models = strings.Split(modelsFlag, ",")
	}

	if len(cfg.models) == 0 {
		log.Fatal("no models available")
	}

	agent := &agent{
		cfg:    cfg,
		keys:   kp,
		ollama: ollama,
		http:   &http.Client{Timeout: 30 * time.Second},
	}

	// Register with gateway
	if err := agent.register(context.Background()); err != nil {
		log.Fatalf("registration failed: %v", err)
	}

	// Run poll loop
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	agent.run(ctx)
}

type agent struct {
	cfg    config
	keys   *auth.KeyPair
	ollama *inference.Client
	http   *http.Client
}

func (a *agent) register(ctx context.Context) error {
	body, _ := json.Marshal(map[string]interface{}{
		"name":       a.cfg.nodeName,
		"public_key": a.keys.PublicKey,
		"models":     a.cfg.models,
	})

	resp, err := a.http.Post(a.cfg.gatewayURL+"/api/nodes/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("register request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		log.Println("node already registered, continuing")
		return nil
	}
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register failed (%d): %s", resp.StatusCode, string(respBody))
	}

	log.Println("registered with gateway — status: pending")
	log.Println("waiting for admin approval before accepting tasks")
	return nil
}

func (a *agent) run(ctx context.Context) {
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	pollTicker := time.NewTicker(a.cfg.pollInterval)
	defer pollTicker.Stop()

	// Initial heartbeat
	a.heartbeat(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		case <-heartbeatTicker.C:
			a.heartbeat(ctx)
		case <-pollTicker.C:
			a.pollAndProcess(ctx)
		}
	}
}

func (a *agent) heartbeat(ctx context.Context) {
	resp, err := a.signedRequest(ctx, "POST", "/api/nodes/heartbeat", nil)
	if err != nil {
		log.Printf("heartbeat failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		log.Println("node not yet approved — waiting")
		return
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("heartbeat error (%d): %s", resp.StatusCode, string(body))
	}
}

func (a *agent) pollAndProcess(ctx context.Context) {
	resp, err := a.signedRequest(ctx, "GET", "/api/tasks/next", nil)
	if err != nil {
		log.Printf("poll failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return // No tasks
	}
	if resp.StatusCode == http.StatusForbidden {
		return // Not approved yet
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("poll error (%d): %s", resp.StatusCode, string(body))
		return
	}

	var task struct {
		ID        string              `json:"id"`
		Model     string              `json:"model"`
		Messages  []inference.Message `json:"messages"`
		MaxTokens int                 `json:"max_tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		log.Printf("decode task failed: %v", err)
		return
	}

	log.Printf("processing task %s (model: %s)", task.ID, task.Model)

	// Run inference against local Ollama
	result, err := a.ollama.Chat(ctx, task.Model, task.Messages)
	if err != nil {
		log.Printf("inference failed for task %s: %v", task.ID, err)
		a.failTask(ctx, task.ID)
		return
	}

	// Submit result
	a.completeTask(ctx, task.ID, result)
}

func (a *agent) completeTask(ctx context.Context, taskID string, result *inference.Result) {
	body, _ := json.Marshal(map[string]interface{}{
		"content":     result.Content,
		"tokens_used": result.TokensUsed,
	})

	resp, err := a.signedRequest(ctx, "POST", "/api/tasks/"+taskID+"/result", body)
	if err != nil {
		log.Printf("complete task %s failed: %v", taskID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Printf("task %s completed (%d tokens)", taskID, result.TokensUsed)
	} else {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("complete task %s error (%d): %s", taskID, resp.StatusCode, string(respBody))
	}
}

func (a *agent) failTask(ctx context.Context, taskID string) {
	resp, err := a.signedRequest(ctx, "POST", "/api/tasks/"+taskID+"/fail", nil)
	if err != nil {
		log.Printf("fail task %s error: %v", taskID, err)
		return
	}
	defer resp.Body.Close()
	log.Printf("task %s returned to queue", taskID)
}

func (a *agent) signedRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	ts := auth.Timestamp()
	sig, err := auth.Sign(a.keys.PrivateKey, method, path, ts)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, a.cfg.gatewayURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Node-PublicKey", a.keys.PublicKey)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Signature", sig)
	req.Header.Set("Content-Type", "application/json")

	return a.http.Do(req)
}
