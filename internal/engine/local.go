package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"omyang/internal/config"
	"omyang/internal/types"
)

// LocalEngine adalah mesin inferensi lokal berbasis llama-server (API OpenAI-compatible).
type LocalEngine struct {
	baseURL string
	model   string
	bin     string
	host    string
	port    int
	timeout time.Duration
	client  *http.Client
	cmd     *exec.Cmd
}

// NewLocalEngine menyiapkan klien HTTP lokal tanpa menjalankan subprocess.
func NewLocalEngine(cfg *config.Config) *LocalEngine {
	host := cfg.Engine.LocalHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Engine.LocalPort
	if port <= 0 {
		port = 8080
	}
	bin := cfg.Engine.LlamaServerBin
	if bin == "" {
		bin = "llama-server"
	}
	timeout := time.Duration(cfg.Engine.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = types.DefaultTimeoutSeconds * time.Second
	}

	return &LocalEngine{
		baseURL: "http://" + host + ":" + strconv.Itoa(port),
		model:   cfg.Engine.LocalModelPath,
		bin:     bin,
		host:    host,
		port:    port,
		timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

// Name mengembalikan tipe engine.
func (e *LocalEngine) Name() string { return types.EngineLocal }

// Start menjalankan llama-server lalu menunggu /health siap.
func (e *LocalEngine) Start() error {
	cmd := exec.Command(e.bin,
		"-m", e.model,
		"--host", e.host,
		"--port", strconv.Itoa(e.port),
	)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("gagal menjalankan %s: %w", e.bin, err)
	}
	e.cmd = cmd

	deadline := time.Now().Add(e.timeout)
	for {
		if err := e.Health(); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			_ = e.Close()
			return fmt.Errorf("llama-server tidak siap di %s setelah %s", e.baseURL, e.timeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Health memeriksa endpoint GET /health.
func (e *LocalEngine) Health() error {
	resp, err := e.client.Get(e.baseURL + "/health")
	if err != nil {
		return fmt.Errorf("llama-server tidak dapat dihubungi: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("llama-server /health membalas status %d", resp.StatusCode)
	}
	return nil
}

// Generate mengirim percakapan ke POST /v1/chat/completions dan memvalidasi respons.
func (e *LocalEngine) Generate(ctx context.Context, messages []types.Message, allowedActions []string) (*types.AgentResponse, error) {
	payload, err := json.Marshal(localRequest{
		Model:    "local",
		Messages: messages,
		Stream:   false,
		ResponseFormat: localResponseFormat{
			Type:   "json_object",
			Schema: actionSchema(allowedActions),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("gagal menyusun request lokal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("gagal membuat request lokal: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal memanggil llama-server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("llama-server membalas status %d: %s", resp.StatusCode, string(detail))
	}

	var out localResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("gagal decode respons llama-server: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("llama-server tidak mengembalikan choices")
	}

	return ParseAndValidateResponse(out.Choices[0].Message.Content, allowedActions)
}

// Close mematikan subprocess secara graceful: SIGTERM, tunggu 3 detik, lalu SIGKILL.
func (e *LocalEngine) Close() error {
	if e.cmd == nil || e.cmd.Process == nil {
		return nil
	}

	_ = e.cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- e.cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = e.cmd.Process.Kill()
		<-done
	}

	e.cmd = nil
	return nil
}

type localRequest struct {
	Model          string              `json:"model"`
	Messages       []types.Message     `json:"messages"`
	Stream         bool                `json:"stream"`
	ResponseFormat localResponseFormat `json:"response_format"`
}

type localResponseFormat struct {
	Type   string         `json:"type"`
	Schema map[string]any `json:"schema"`
}

type localResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
