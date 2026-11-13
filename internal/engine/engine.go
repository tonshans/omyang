package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"omyang/internal/config"
	"omyang/internal/sanitizer"
	"omyang/internal/types"
)

// Engine adalah antarmuka mesin inferensi (lokal maupun cloud).
type Engine interface {
	Name() string
	Generate(ctx context.Context, messages []types.Message, allowedActions []string) (*types.AgentResponse, error)
	Close() error
}

// ParseAndValidateResponse mengekstrak JSON dari raw dan memastikan action termasuk allowedActions.
func ParseAndValidateResponse(raw string, allowedActions []string) (*types.AgentResponse, error) {
	jsonText, err := sanitizer.ExtractJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("respons model tidak memuat JSON valid: %w", err)
	}

	var resp types.AgentResponse
	if err := json.Unmarshal([]byte(jsonText), &resp); err != nil {
		return nil, fmt.Errorf("gagal parse JSON respons: %w", err)
	}

	for _, allowed := range allowedActions {
		if resp.Action == allowed {
			return &resp, nil
		}
	}
	return nil, fmt.Errorf("aksi %q tidak diizinkan (allowed: %s)", resp.Action, strings.Join(allowedActions, ", "))
}

// actionSchema adalah skema JSON ketat untuk thought/action/input menurut allowedActions.
func actionSchema(allowedActions []string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"thought": map[string]any{"type": "string"},
			"action":  map[string]any{"type": "string", "enum": allowedActions},
			"input":   map[string]any{"type": "string"},
		},
		"required": []string{"thought", "action", "input"},
	}
}

// CircuitBreakerMonitor mendeteksi multi-hop failure pada Orkestrator.
type CircuitBreakerMonitor struct {
	maxSteps                int
	minThoughtLen           int
	maxDuplicateDelegations int
	lastThought             string
	delegations             map[string]int
}

// NewCircuitBreakerMonitor membuat monitor dari konfigurasi, dengan fallback ke nilai default sistem.
func NewCircuitBreakerMonitor(cb config.CircuitBreakers) *CircuitBreakerMonitor {
	m := &CircuitBreakerMonitor{
		maxSteps:                cb.OrchestratorMaxSteps,
		minThoughtLen:           cb.MinThoughtLength,
		maxDuplicateDelegations: cb.MaxDuplicateDelegations,
		delegations:             make(map[string]int),
	}
	if m.maxSteps <= 0 {
		m.maxSteps = types.DefaultOrchMaxSteps
	}
	if m.minThoughtLen <= 0 {
		m.minThoughtLen = types.DefaultMinThoughtLength
	}
	if m.maxDuplicateDelegations <= 0 {
		m.maxDuplicateDelegations = types.DefaultMaxDuplicateDelegation
	}
	return m
}

// CheckOrchestratorStep menjalankan Early Exhaustion, Thought Density, dan Circular Delegation.
func (m *CircuitBreakerMonitor) CheckOrchestratorStep(step int, resp *types.AgentResponse) error {
	if resp == nil {
		return fmt.Errorf("circuit breaker: respons Orkestrator kosong. Beralih ke /engine cloud")
	}

	// 1. Early Exhaustion
	if step > m.maxSteps && resp.Action != types.ActionFinalAnswer {
		return fmt.Errorf("circuit breaker: Orkestrator melebihi %d langkah tanpa final_answer. Beralih ke /engine cloud", m.maxSteps)
	}

	// 2. Thought Density
	thought := strings.TrimSpace(resp.Thought)
	if len(thought) < m.minThoughtLen || thought == m.lastThought {
		return fmt.Errorf("circuit breaker: thought terlalu pendek atau identik dengan langkah sebelumnya. Beralih ke /engine cloud")
	}
	m.lastThought = thought

	// 3. Circular Delegation
	if resp.Action == types.ActionDelegate {
		key := normalizeDelegationInput(resp.Input)
		m.delegations[key]++
		if m.delegations[key] >= m.maxDuplicateDelegations {
			return fmt.Errorf("circuit breaker: delegasi %q berulang %d kali. Beralih ke /engine cloud", key, m.delegations[key])
		}
	}

	return nil
}

// normalizeDelegationInput menyeragamkan input delegasi agar duplikat terdeteksi.
func normalizeDelegationInput(input string) string {
	return strings.Join(strings.Fields(strings.ToLower(input)), " ")
}
