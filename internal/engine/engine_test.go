package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"omyang/internal/config"
	"omyang/internal/types"
)

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("gagal decode body request: %v", err)
	}
	return body
}

func asMap(t *testing.T, v any, path string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s bukan object JSON: %#v", path, v)
	}
	return m
}

func asSlice(t *testing.T, v any, path string) []any {
	t.Helper()
	s, ok := v.([]any)
	if !ok {
		t.Fatalf("%s bukan array JSON: %#v", path, v)
	}
	return s
}

func TestParseAndValidateResponse(t *testing.T) {
	allowed := []string{types.ActionBash, types.ActionReportDone}

	raw := "Berikut hasilnya:\n```json\n{\"thought\":\"ok\",\"action\":\"bash\",\"input\":\"ls\"}\n```"
	resp, err := ParseAndValidateResponse(raw, allowed)
	if err != nil {
		t.Fatalf("respons valid harus lolos: %v", err)
	}
	if resp.Thought != "ok" || resp.Action != types.ActionBash || resp.Input != "ls" {
		t.Errorf("respons tidak sesuai kontrak: %#v", resp)
	}

	if _, err := ParseAndValidateResponse(`{"thought":"ok","action":"write","input":"x"}`, allowed); err == nil {
		t.Error("aksi di luar allowedActions wajib ditolak")
	}

	if _, err := ParseAndValidateResponse("tidak ada json sama sekali", allowed); err == nil {
		t.Error("output tanpa JSON wajib ditolak")
	}

	if _, err := ParseAndValidateResponse(`{"thought":"ok","action":"bash","input":"ls"}`, nil); err == nil {
		t.Error("allowedActions kosong wajib menolak semua aksi")
	}
}

func TestCircuitBreakerCircularDelegation(t *testing.T) {
	m := NewCircuitBreakerMonitor(config.CircuitBreakers{MaxDuplicateDelegations: 2})

	first := &types.AgentResponse{
		Thought: "Saya delegasikan pemeriksaan log.",
		Action:  types.ActionDelegate,
		Input:   "cek log",
	}
	if err := m.CheckOrchestratorStep(1, first); err != nil {
		t.Fatalf("delegasi pertama harus lolos: %v", err)
	}

	// Input sama setelah normalisasi spasi harus dihitung sebagai duplikat.
	second := &types.AgentResponse{
		Thought: "Saya ulangi delegasi pemeriksaan log.",
		Action:  types.ActionDelegate,
		Input:   "  cek   log ",
	}
	err := m.CheckOrchestratorStep(2, second)
	if err == nil {
		t.Fatal("delegasi identik kedua wajib memicu Circuit Breaker")
	}
	if !strings.Contains(err.Error(), "/engine cloud") {
		t.Errorf("error wajib menyarankan /engine cloud, dapat: %q", err.Error())
	}

	// Input berbeda tidak boleh dianggap duplikat.
	other := NewCircuitBreakerMonitor(config.CircuitBreakers{})
	if err := other.CheckOrchestratorStep(1, first); err != nil {
		t.Fatalf("delegasi pertama harus lolos: %v", err)
	}
	if err := other.CheckOrchestratorStep(2, &types.AgentResponse{
		Thought: "Sekarang saya periksa kapasitas disk.",
		Action:  types.ActionDelegate,
		Input:   "cek disk",
	}); err != nil {
		t.Errorf("delegasi dengan input berbeda tidak boleh diblokir: %v", err)
	}
}

func TestCircuitBreakerEarlyExhaustion(t *testing.T) {
	m := NewCircuitBreakerMonitor(config.CircuitBreakers{OrchestratorMaxSteps: 2})

	// Input tiap langkah sengaja berbeda agar yang diuji murni Early Exhaustion,
	// bukan Circular Delegation.
	if err := m.CheckOrchestratorStep(2, &types.AgentResponse{
		Thought: "Langkah kedua masih dalam batas.",
		Action:  types.ActionDelegate,
		Input:   "cek status sistem",
	}); err != nil {
		t.Fatalf("step == maxSteps harus lolos: %v", err)
	}
	err := m.CheckOrchestratorStep(3, &types.AgentResponse{
		Thought: "Langkah ketiga melewati batas.",
		Action:  types.ActionDelegate,
		Input:   "cek penggunaan memori",
	})
	if err == nil {
		t.Fatal("step > maxSteps tanpa final_answer wajib memicu Circuit Breaker")
	}
	if !strings.Contains(err.Error(), "/engine cloud") {
		t.Errorf("error wajib menyarankan /engine cloud, dapat: %q", err.Error())
	}

	// final_answer tidak pernah dianggap early exhaustion.
	last := NewCircuitBreakerMonitor(config.CircuitBreakers{OrchestratorMaxSteps: 2})
	if err := last.CheckOrchestratorStep(9, &types.AgentResponse{
		Thought: "Saya sudah menyelesaikan seluruh tugas.",
		Action:  types.ActionFinalAnswer,
		Input:   "selesai",
	}); err != nil {
		t.Errorf("final_answer pada step berapa pun harus lolos: %v", err)
	}
}

func TestCircuitBreakerThoughtDensity(t *testing.T) {
	m := NewCircuitBreakerMonitor(config.CircuitBreakers{MinThoughtLength: 15})

	if err := m.CheckOrchestratorStep(1, &types.AgentResponse{
		Thought: "oke",
		Action:  types.ActionFinalAnswer,
	}); err == nil {
		t.Fatal("thought lebih pendek dari minThoughtLength wajib memicu Circuit Breaker")
	}

	long := "Saya akan memeriksa isi direktori workspace."
	if err := m.CheckOrchestratorStep(2, &types.AgentResponse{
		Thought: long,
		Action:  types.ActionFinalAnswer,
		Input:   "langkah satu",
	}); err != nil {
		t.Fatalf("thought panjang harus lolos: %v", err)
	}

	if err := m.CheckOrchestratorStep(3, &types.AgentResponse{
		Thought: long,
		Action:  types.ActionFinalAnswer,
		Input:   "langkah dua",
	}); err == nil {
		t.Fatal("thought identik dengan langkah sebelumnya wajib memicu Circuit Breaker")
	}

	if err := m.CheckOrchestratorStep(4, &types.AgentResponse{
		Thought: "Saya akan memeriksa kapasitas disk perangkat.",
		Action:  types.ActionFinalAnswer,
		Input:   "langkah tiga",
	}); err != nil {
		t.Errorf("thought baru yang panjang harus lolos: %v", err)
	}
}

func TestCircuitBreakerDefaults(t *testing.T) {
	m := NewCircuitBreakerMonitor(config.CircuitBreakers{})
	if m.maxSteps != types.DefaultOrchMaxSteps {
		t.Errorf("maxSteps default = %d, want %d", m.maxSteps, types.DefaultOrchMaxSteps)
	}
	if m.minThoughtLen != types.DefaultMinThoughtLength {
		t.Errorf("minThoughtLen default = %d, want %d", m.minThoughtLen, types.DefaultMinThoughtLength)
	}
	if m.maxDuplicateDelegations != types.DefaultMaxDuplicateDelegation {
		t.Errorf("maxDuplicateDelegations default = %d, want %d", m.maxDuplicateDelegations, types.DefaultMaxDuplicateDelegation)
	}
}

func TestLocalEngineGenerate(t *testing.T) {
	var got map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request tak terduga: %s %s", r.Method, r.URL.Path)
		}
		got = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{\"thought\":\"Saya cek log dulu.\",\"action\":\"delegate_task\",\"input\":\"cek log\"}"}}]}`)
	}))
	defer srv.Close()

	eng := NewLocalEngine(&config.Config{Engine: config.EngineSettings{
		LocalHost:      "127.0.0.1",
		LocalPort:      1,
		TimeoutSeconds: 5,
	}})
	eng.baseURL = srv.URL

	if eng.Name() != types.EngineLocal {
		t.Errorf("Name() = %q, want %q", eng.Name(), types.EngineLocal)
	}

	messages := []types.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "halo"},
	}
	resp, err := eng.Generate(context.Background(), messages, []string{types.ActionDelegate, types.ActionFinalAnswer})
	if err != nil {
		t.Fatalf("Generate gagal: %v", err)
	}
	if resp.Action != types.ActionDelegate || resp.Input != "cek log" {
		t.Errorf("respons tidak tervalidasi dengan benar: %#v", resp)
	}

	// Kontrak payload: messages diteruskan apa adanya.
	rawMsgs := asSlice(t, got["messages"], "messages")
	if len(rawMsgs) != 2 {
		t.Fatalf("messages harus 2, dapat %d", len(rawMsgs))
	}
	first := asMap(t, rawMsgs[0], "messages[0]")
	if first["role"] != "system" || first["content"] != "sys" {
		t.Errorf("messages[0] tidak sesuai: %#v", first)
	}

	// Kontrak payload: response_format json_object + skema thought/action/input.
	rf := asMap(t, got["response_format"], "response_format")
	if rf["type"] != "json_object" {
		t.Errorf("response_format.type = %#v, want json_object", rf["type"])
	}
	schema := asMap(t, rf["schema"], "response_format.schema")
	props := asMap(t, schema["properties"], "response_format.schema.properties")
	for _, key := range []string{"thought", "action", "input"} {
		if _, ok := props[key]; !ok {
			t.Errorf("skema tidak memuat properti %q", key)
		}
	}
	action := asMap(t, props["action"], "properties.action")
	enum := asSlice(t, action["enum"], "properties.action.enum")
	if len(enum) != 2 || enum[0] != types.ActionDelegate || enum[1] != types.ActionFinalAnswer {
		t.Errorf("enum action tidak sesuai allowedActions: %#v", enum)
	}
}

func TestLocalEngineHTTPErrors(t *testing.T) {
	mode := "500"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch mode {
		case "500":
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, "boom")
		case "badjson":
			io.WriteString(w, `{"choices":[{"message":{"content":"bukan json"}}]}`)
		case "empty":
			io.WriteString(w, `{"choices":[]}`)
		}
	}))
	defer srv.Close()

	eng := NewLocalEngine(&config.Config{})
	eng.baseURL = srv.URL

	if err := eng.Health(); err == nil {
		t.Error("Health() wajib error saat /health bukan 200")
	}

	if _, err := eng.Generate(context.Background(), nil, []string{types.ActionFinalAnswer}); err == nil {
		t.Error("Generate wajib error saat server membalas 500")
	}

	mode = "badjson"
	if _, err := eng.Generate(context.Background(), nil, []string{types.ActionFinalAnswer}); err == nil {
		t.Error("Generate wajib error saat konten bukan JSON")
	}

	mode = "empty"
	if _, err := eng.Generate(context.Background(), nil, []string{types.ActionFinalAnswer}); err == nil {
		t.Error("Generate wajib error saat choices kosong")
	}
}

func TestLocalEngineHealthOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path tak terduga: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	eng := NewLocalEngine(&config.Config{})
	eng.baseURL = srv.URL
	if err := eng.Health(); err != nil {
		t.Errorf("Health() harus nil saat /health 200: %v", err)
	}
}

func TestLocalEngineClose(t *testing.T) {
	// Tanpa subprocess: Close harus no-op.
	eng := NewLocalEngine(&config.Config{})
	if err := eng.Close(); err != nil {
		t.Errorf("Close tanpa subprocess harus nil, dapat: %v", err)
	}

	// Subprocess yang patuh SIGTERM: Close harus mematikannya dan menunggu Wait.
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("gagal start proses uji: %v", err)
	}
	eng.cmd = cmd
	if err := eng.Close(); err != nil {
		t.Fatalf("Close gagal: %v", err)
	}
	if cmd.ProcessState == nil {
		t.Error("Close harus menunggu subprocess selesai (Wait)")
	}
	if cmd.Process.Signal(syscall.Signal(0)) == nil {
		t.Error("proses llama-server harus sudah mati setelah Close")
	}
	if eng.cmd != nil {
		t.Error("Close harus melepas referensi subprocess")
	}
}

func TestCloudEngineGenerate(t *testing.T) {
	var got map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method tak terduga: %s", r.Method)
		}
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash:generateContent" {
			t.Errorf("path tak terduga: %s", r.URL.Path)
		}
		if key := r.URL.Query().Get("key"); key != "test-key" {
			t.Errorf("query key = %q, want test-key", key)
		}
		got = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"{\"thought\":\"Saya baca file log.\",\"action\":\"bash\",\"input\":\"cat log\"}"}]}}]}`)
	}))
	defer srv.Close()

	eng := NewCloudEngine(&config.Config{Engine: config.EngineSettings{
		CloudAPIKey:    "test-key",
		CloudModel:     "gemini-2.5-flash",
		TimeoutSeconds: 5,
	}})
	eng.baseURL = srv.URL

	if eng.Name() != types.EngineCloud {
		t.Errorf("Name() = %q, want %q", eng.Name(), types.EngineCloud)
	}
	if err := eng.Close(); err != nil {
		t.Errorf("Close cloud harus nil: %v", err)
	}

	messages := []types.Message{
		{Role: "system", Content: "sys-prompt"},
		{Role: "user", Content: "halo"},
		{Role: "assistant", Content: "hai"},
		{Role: "user", Content: "baca log"},
	}
	resp, err := eng.Generate(context.Background(), messages, []string{types.ActionBash, types.ActionReportDone})
	if err != nil {
		t.Fatalf("Generate gagal: %v", err)
	}
	if resp.Action != types.ActionBash || resp.Input != "cat log" {
		t.Errorf("respons tidak tervalidasi dengan benar: %#v", resp)
	}

	// System prompt dipisah ke systemInstruction, bukan contents.
	sys := asMap(t, got["systemInstruction"], "systemInstruction")
	sysParts := asSlice(t, sys["parts"], "systemInstruction.parts")
	if len(sysParts) != 1 || asMap(t, sysParts[0], "systemInstruction.parts[0]")["text"] != "sys-prompt" {
		t.Errorf("systemInstruction tidak sesuai: %#v", sys)
	}

	contents := asSlice(t, got["contents"], "contents")
	if len(contents) != 3 {
		t.Fatalf("contents harus 3 (user, model, user), dapat %d", len(contents))
	}
	if asMap(t, contents[0], "contents[0]")["role"] != "user" {
		t.Error("pesan user pertama wajib role user")
	}
	if asMap(t, contents[1], "contents[1]")["role"] != "model" {
		t.Error("pesan assistant wajib dipetakan ke role model")
	}

	// Kontrak generationConfig.
	gc := asMap(t, got["generationConfig"], "generationConfig")
	if gc["responseMimeType"] != "application/json" {
		t.Errorf("responseMimeType = %#v, want application/json", gc["responseMimeType"])
	}
	schema := asMap(t, gc["responseSchema"], "generationConfig.responseSchema")
	props := asMap(t, schema["properties"], "responseSchema.properties")
	action := asMap(t, props["action"], "properties.action")
	enum := asSlice(t, action["enum"], "properties.action.enum")
	if len(enum) != 2 || enum[0] != types.ActionBash || enum[1] != types.ActionReportDone {
		t.Errorf("enum action tidak sesuai allowedActions: %#v", enum)
	}
}

func TestCloudEngineHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()

	eng := NewCloudEngine(&config.Config{Engine: config.EngineSettings{CloudAPIKey: "x"}})
	eng.baseURL = srv.URL

	if _, err := eng.Generate(context.Background(), []types.Message{{Role: "user", Content: "hi"}}, []string{types.ActionFinalAnswer}); err == nil {
		t.Error("Generate cloud wajib error saat server membalas 400")
	}
}

func TestCloudEngineMissingCandidates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"candidates":[]}`)
	}))
	defer srv.Close()

	eng := NewCloudEngine(&config.Config{Engine: config.EngineSettings{CloudAPIKey: "x"}})
	eng.baseURL = srv.URL

	if _, err := eng.Generate(context.Background(), nil, []string{types.ActionFinalAnswer}); err == nil {
		t.Error("Generate cloud wajib error saat candidates kosong")
	}
}
