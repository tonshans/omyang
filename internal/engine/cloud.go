package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"omyang/internal/config"
	"omyang/internal/types"
)

// CloudEngine adalah mesin inferensi awan berbasis Google Gemini Flash.
type CloudEngine struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

// NewCloudEngine menyiapkan klien HTTP Gemini dari konfigurasi.
func NewCloudEngine(cfg *config.Config) *CloudEngine {
	model := cfg.Engine.CloudModel
	if model == "" {
		model = "gemini-2.5-flash"
	}
	timeout := time.Duration(cfg.Engine.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = types.DefaultTimeoutSeconds * time.Second
	}

	return &CloudEngine{
		baseURL: "https://generativelanguage.googleapis.com",
		model:   model,
		apiKey:  cfg.Engine.CloudAPIKey,
		client:  &http.Client{Timeout: timeout},
	}
}

// Name mengembalikan tipe engine.
func (e *CloudEngine) Name() string { return types.EngineCloud }

// Close tidak memegang resource apa pun.
func (e *CloudEngine) Close() error { return nil }

// Generate memanggil generateContent Gemini dan memvalidasi respons JSON.
func (e *CloudEngine) Generate(ctx context.Context, messages []types.Message, allowedActions []string) (*types.AgentResponse, error) {
	payload, err := json.Marshal(buildGeminiRequest(messages, allowedActions))
	if err != nil {
		return nil, fmt.Errorf("gagal menyusun request cloud: %w", err)
	}

	endpoint := e.baseURL + "/v1beta/models/" + e.model + ":generateContent?key=" + url.QueryEscape(e.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("gagal membuat request cloud: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal memanggil Gemini: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Gemini membalas status %d: %s", resp.StatusCode, string(detail))
	}

	var out geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("gagal decode respons Gemini: %w", err)
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("Gemini tidak mengembalikan kandidat respons")
	}

	return ParseAndValidateResponse(out.Candidates[0].Content.Parts[0].Text, allowedActions)
}

// buildGeminiRequest memetakan pesan ke contents Gemini dan memisahkan system prompt.
func buildGeminiRequest(messages []types.Message, allowedActions []string) geminiRequest {
	req := geminiRequest{
		GenerationConfig: geminiGenerationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema:   actionSchema(allowedActions),
		},
	}

	var systemParts []geminiPart
	for _, msg := range messages {
		if msg.Role == "system" {
			systemParts = append(systemParts, geminiPart{Text: msg.Content})
			continue
		}
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}
		req.Contents = append(req.Contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Content}},
		})
	}

	if len(systemParts) > 0 {
		req.SystemInstruction = &geminiContent{Parts: systemParts}
	}
	return req
}

type geminiRequest struct {
	Contents          []geminiContent        `json:"contents"`
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	ResponseMimeType string         `json:"responseMimeType"`
	ResponseSchema   map[string]any `json:"responseSchema"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
}
