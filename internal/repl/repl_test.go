package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"omyang/internal/config"
	"omyang/internal/engine"
	"omyang/internal/types"
)

type engineCall struct {
	messages []types.Message
	allowed  []string
}

type mockEngine struct {
	mu      sync.Mutex
	calls   []engineCall
	handler func(idx int, call engineCall) (*types.AgentResponse, error)
}

func (m *mockEngine) Name() string { return "mock" }

func (m *mockEngine) Close() error { return nil }

func (m *mockEngine) Generate(_ context.Context, messages []types.Message, allowedActions []string) (*types.AgentResponse, error) {
	m.mu.Lock()
	call := engineCall{
		messages: append([]types.Message(nil), messages...),
		allowed:  append([]string(nil), allowedActions...),
	}
	idx := len(m.calls)
	m.calls = append(m.calls, call)
	handler := m.handler
	m.mu.Unlock()

	if handler == nil {
		return &types.AgentResponse{
			Thought: "Jawaban akhir pengujian yang cukup panjang.",
			Action:  types.ActionFinalAnswer,
			Input:   "SELESAI",
		}, nil
	}
	return handler(idx, call)
}

func (m *mockEngine) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockEngine) Call(idx int) engineCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[idx]
}

func testRoles() map[string]types.RoleProfile {
	return map[string]types.RoleProfile{
		types.DefaultRoleName: {
			SystemPrompt: "DEFAULT_ROLE_PROMPT",
			FewShot: []types.Message{
				{Role: "user", Content: "DEFAULT_FEWSHOT"},
			},
		},
		"sysadmin": {
			SystemPrompt: "SYSADMIN_ROLE_PROMPT",
		},
	}
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	baseDir := t.TempDir()
	workspace := filepath.Join(baseDir, "workspace")
	skills := filepath.Join(baseDir, "skills")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("gagal membuat workspace: %v", err)
	}
	if err := os.MkdirAll(skills, 0755); err != nil {
		t.Fatalf("gagal membuat skills: %v", err)
	}

	return &config.Config{
		Engine: config.EngineSettings{
			ActiveEngine:   types.EngineLocal,
			TimeoutSeconds: 5,
		},
		Memory: config.MemoryManagement{
			OrchMaxHistoryPairs:       6,
			MacroCompressionThreshold: 5,
			TruncationHeadChars:       100,
			TruncationTailChars:       100,
			MaxSkillChars:             4000,
		},
		CircuitBreaker: config.CircuitBreakers{
			OrchestratorMaxSteps:    4,
			WorkerMaxSteps:          3,
			MinThoughtLength:        15,
			MaxDuplicateDelegations: 2,
		},
		Security: config.SecuritySettings{
			DefaultMode:        types.ModeAuto,
			AllowedDirectories: []string{workspace},
		},
		ModePrompts: config.ModePrompts{
			Talk: "TALK_MODE_PROMPT",
			Plan: "PLAN_MODE_PROMPT",
			Work: "WORK_MODE_PROMPT",
		},
		WorkerPrompt: config.WorkerPromptCfg{
			SystemPrompt: "WORKER_SYSTEM_PROMPT",
		},
		DefaultSession:   types.SessionTalk,
		DefaultRole:      types.DefaultRoleName,
		BaseDir:          baseDir,
		SessionStatePath: filepath.Join(baseDir, ".session_state.json"),
		SkillsDir:        skills,
		WorkspaceDir:     workspace,
	}
}

func newTestSession(t *testing.T, cfg *config.Config, eng engine.Engine) (*Session, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	s := NewSession(cfg, testRoles(), eng, nil, strings.NewReader(""), out)
	return s, out
}

func TestFormatStatusBarHidesSecurityOutsideWorkMode(t *testing.T) {
	s, _ := newTestSession(t, testConfig(t), &mockEngine{})

	if handled, _ := s.HandleCommand("/talk"); !handled {
		t.Fatal("/talk harus dikenali")
	}
	bar := s.FormatStatusBar()
	for _, want := range []string{"💻 Local", "💬 Talk", "🎭 default", "⚡ -"} {
		if !strings.Contains(bar, want) {
			t.Errorf("status bar /talk harus memuat %q, dapat %q", want, bar)
		}
	}
	for _, banned := range []string{"🛡️", "⚠️", "Copilot", "Auto"} {
		if strings.Contains(bar, banned) {
			t.Errorf("status bar /talk tidak boleh memuat indikator keamanan %q: %q", banned, bar)
		}
	}

	if handled, _ := s.HandleCommand("/plan"); !handled {
		t.Fatal("/plan harus dikenali")
	}
	bar = s.FormatStatusBar()
	if !strings.Contains(bar, "🧠 Plan") {
		t.Errorf("status bar /plan harus memuat 🧠 Plan: %q", bar)
	}
	if strings.Contains(bar, "🛡️") || strings.Contains(bar, "⚠️") {
		t.Errorf("status bar /plan tidak boleh memuat indikator keamanan: %q", bar)
	}

	if handled, _ := s.HandleCommand("/work"); !handled {
		t.Fatal("/work harus dikenali")
	}
	if handled, _ := s.HandleCommand("/mode copilot"); !handled {
		t.Fatal("/mode copilot harus dikenali")
	}
	bar = s.FormatStatusBar()
	for _, want := range []string{"🚀 Work", "🛡️ Copilot"} {
		if !strings.Contains(bar, want) {
			t.Errorf("status bar /work copilot harus memuat %q, dapat %q", want, bar)
		}
	}

	if handled, _ := s.HandleCommand("/mode auto"); !handled {
		t.Fatal("/mode auto harus dikenali")
	}
	bar = s.FormatStatusBar()
	if !strings.Contains(bar, "⚠️ Auto") {
		t.Errorf("status bar mode auto harus memuat ⚠️ Auto: %q", bar)
	}
}

func TestRoleplayCommandInjectsCustomPersona(t *testing.T) {
	s, _ := newTestSession(t, testConfig(t), &mockEngine{})

	handled, exit := s.HandleCommand("/roleplay kamu adalah sysadmin 1964")
	if !handled || exit {
		t.Fatalf("/roleplay harus ditangani tanpa keluar, handled=%v exit=%v", handled, exit)
	}
	if s.activeRoleName != types.CustomRoleName {
		t.Errorf("activeRoleName = %q, want %q", s.activeRoleName, types.CustomRoleName)
	}
	if !strings.Contains(s.FormatStatusBar(), "🎭 custom*") {
		t.Errorf("status bar harus menampilkan persona kustom: %q", s.FormatStatusBar())
	}
	if prompt := s.BuildSystemPrompt(); !strings.Contains(prompt, "kamu adalah sysadmin 1964") {
		t.Errorf("system prompt harus memuat persona kustom, dapat:\n%s", prompt)
	}
}

func TestSlashCommandsSessionState(t *testing.T) {
	cfg := testConfig(t)
	s, out := newTestSession(t, cfg, &mockEngine{})

	if _, _ = s.HandleCommand("/work"); s.sessionMode != types.SessionWork {
		t.Errorf("sessionMode = %q, want work", s.sessionMode)
	}
	if _, _ = s.HandleCommand("/engine cloud"); s.activeEngineType != types.EngineCloud {
		t.Errorf("activeEngineType = %q, want cloud", s.activeEngineType)
	}
	if !strings.Contains(s.FormatStatusBar(), "☁️ Cloud") {
		t.Errorf("status bar harus menampilkan Cloud: %q", s.FormatStatusBar())
	}

	cfg.Engine.ActiveEngine = types.EngineLocal
	if _, _ = s.HandleCommand("/engine local"); s.activeEngineType != types.EngineLocal {
		t.Errorf("activeEngineType = %q, want local", s.activeEngineType)
	}

	if _, _ = s.HandleCommand("/role sysadmin"); s.activeRoleName != "sysadmin" {
		t.Errorf("activeRoleName = %q, want sysadmin", s.activeRoleName)
	}
	if !strings.Contains(s.BuildSystemPrompt(), "SYSADMIN_ROLE_PROMPT") {
		t.Error("system prompt harus memakai profil role sysadmin")
	}

	out.Reset()
	if _, _ = s.HandleCommand("/role"); !strings.Contains(out.String(), "default") || !strings.Contains(out.String(), "sysadmin") {
		t.Errorf("/role tanpa argumen harus mendaftar role, dapat: %q", out.String())
	}
	if _, _ = s.HandleCommand("/role tidak-ada"); !strings.Contains(out.String(), "tidak ditemukan") && !strings.Contains(out.String(), "tidak dikenal") {
		t.Errorf("role tak dikenal harus melaporkan kesalahan, dapat: %q", out.String())
	}

	skillPath := filepath.Join(cfg.SkillsDir, "termux.md")
	if err := os.WriteFile(skillPath, []byte("TERMUX_SKILL_CONTENT"), 0644); err != nil {
		t.Fatalf("gagal menulis skill: %v", err)
	}
	out.Reset()
	if _, _ = s.HandleCommand("/skill"); !strings.Contains(out.String(), "termux") {
		t.Errorf("/skill harus mendaftar skill, dapat: %q", out.String())
	}
	if _, _ = s.HandleCommand("/skill termux"); s.activeSkillName != "termux" {
		t.Errorf("activeSkillName = %q, want termux", s.activeSkillName)
	}
	if !strings.Contains(s.BuildSystemPrompt(), "TERMUX_SKILL_CONTENT") {
		t.Error("system prompt harus memuat isi skill aktif")
	}
	if !strings.Contains(s.FormatStatusBar(), "⚡ termux") {
		t.Errorf("status bar harus menampilkan skill aktif: %q", s.FormatStatusBar())
	}
	if _, _ = s.HandleCommand("/skill clear"); s.activeSkillName != "-" {
		t.Errorf("activeSkillName = %q, want -", s.activeSkillName)
	}
	if strings.Contains(s.BuildSystemPrompt(), "TERMUX_SKILL_CONTENT") {
		t.Error("system prompt tidak boleh memuat skill setelah clear")
	}

	if handled, exit := s.HandleCommand("/exit"); !handled || !exit {
		t.Errorf("/exit harus menghentikan sesi, handled=%v exit=%v", handled, exit)
	}
	if handled, exit := s.HandleCommand("bukan slash command"); handled || exit {
		t.Errorf("input biasa bukan slash command, handled=%v exit=%v", handled, exit)
	}
}

func TestNewCommandClearsHistoryAndStateFile(t *testing.T) {
	cfg := testConfig(t)
	s, _ := newTestSession(t, cfg, &mockEngine{})

	s.HandleCommand("/work")
	s.HandleCommand("/role sysadmin")
	_ = os.WriteFile(filepath.Join(cfg.SkillsDir, "ops.md"), []byte("OPS_SKILL"), 0644)
	s.HandleCommand("/skill ops")
	s.HandleCommand("/engine cloud")

	s.macroMem.AddPair(types.MessagePair{
		UserMsg:      types.Message{Role: "user", Content: "HISTORY_USER"},
		AssistantMsg: types.Message{Role: "assistant", Content: "HISTORY_ASSISTANT"},
	})
	s.saveState()
	if _, err := os.Stat(cfg.SessionStatePath); err != nil {
		t.Fatalf("state file harus dibuat sebelum /new: %v", err)
	}

	s.HandleCommand("/new")

	if _, err := os.Stat(cfg.SessionStatePath); !os.IsNotExist(err) {
		t.Errorf("/new harus menghapus .session_state.json, err=%v", err)
	}
	messages := s.macroMem.BuildMessages(s.sessionMode, "PROMPT_BARU")
	for _, msg := range messages {
		if strings.Contains(msg.Content, "HISTORY_") {
			t.Errorf("/new harus mengosongkan riwayat, bocor: %+v", msg)
		}
	}
	if s.sessionMode != types.SessionWork || s.activeEngineType != types.EngineCloud ||
		s.activeRoleName != "sysadmin" || s.activeSkillName != "ops" {
		t.Errorf("/new harus mempertahankan engine/mode/role/skill, dapat: engine=%q mode=%q role=%q skill=%q",
			s.activeEngineType, s.sessionMode, s.activeRoleName, s.activeSkillName)
	}
	if !strings.Contains(s.BuildSystemPrompt(), "OPS_SKILL") {
		t.Error("/new harus mempertahankan isi skill aktif")
	}
}

func TestResumeRestoresSessionState(t *testing.T) {
	cfg := testConfig(t)
	s, _ := newTestSession(t, cfg, &mockEngine{})

	state := types.SessionState{
		ActiveEngine: types.EngineCloud,
		SecurityMode: types.ModeAuto,
		SessionMode:  types.SessionPlan,
		ActiveRole:   "sysadmin",
		ActiveSkill:  "-",
		ApprovedPlan: "RENCANA_LAMA",
		Pairs: []types.MessagePair{{
			UserMsg:      types.Message{Role: "user", Content: "HISTORY_USER"},
			AssistantMsg: types.Message{Role: "assistant", Content: "HISTORY_ASSISTANT"},
		}},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("gagal marshal state: %v", err)
	}
	if err := os.WriteFile(cfg.SessionStatePath, data, 0644); err != nil {
		t.Fatalf("gagal menulis state: %v", err)
	}

	handled, exit := s.HandleCommand("/resume")
	if !handled || exit {
		t.Fatalf("/resume harus ditangani tanpa keluar, handled=%v exit=%v", handled, exit)
	}
	if s.activeEngineType != types.EngineCloud || s.sessionMode != types.SessionPlan || s.activeRoleName != "sysadmin" {
		t.Errorf("state sesi tidak dipulihkan: engine=%q mode=%q role=%q",
			s.activeEngineType, s.sessionMode, s.activeRoleName)
	}
	bar := s.FormatStatusBar()
	for _, want := range []string{"☁️ Cloud", "🧠 Plan", "🎭 sysadmin"} {
		if !strings.Contains(bar, want) {
			t.Errorf("status bar setelah /resume harus memuat %q: %q", want, bar)
		}
	}
	if !strings.Contains(s.BuildSystemPrompt(), "SYSADMIN_ROLE_PROMPT") {
		t.Error("/resume harus memulihkan role aktif")
	}

	var joined strings.Builder
	for _, msg := range s.macroMem.BuildMessages(s.sessionMode, "PROMPT") {
		joined.WriteString(msg.Content)
		joined.WriteString("\n")
	}
	if !strings.Contains(joined.String(), "HISTORY_USER") || !strings.Contains(joined.String(), "HISTORY_ASSISTANT") {
		t.Errorf("/resume harus memulihkan riwayat Macro-Memory: %s", joined.String())
	}

	// File rusak/tidak ada tidak boleh membuat program panik.
	_ = os.Remove(cfg.SessionStatePath)
	if handled, _ := s.HandleCommand("/resume"); !handled {
		t.Error("/resume tanpa file tetap harus ditangani")
	}
}

func TestBtwDoesNotTouchMacroMemory(t *testing.T) {
	eng := &mockEngine{}
	s, out := newTestSession(t, testConfig(t), eng)
	s.macroMem.AddPair(types.MessagePair{
		UserMsg:      types.Message{Role: "user", Content: "HISTORY_USER"},
		AssistantMsg: types.Message{Role: "assistant", Content: "HISTORY_ASSISTANT"},
	})
	before := len(s.macroMem.BuildMessages(s.sessionMode, "X"))

	s.HandleCommand("/btw apa itu termux?")

	if eng.CallCount() != 1 {
		t.Errorf("/btw harus memanggil engine tepat 1 kali, dapat %d", eng.CallCount())
	}
	if after := len(s.macroMem.BuildMessages(s.sessionMode, "X")); after != before {
		t.Errorf("/btw tidak boleh mengubah Macro-Memory, sebelum=%d sesudah=%d", before, after)
	}
	if !strings.Contains(out.String(), "SELESAI") {
		t.Errorf("/btw harus mencetak jawaban, dapat: %q", out.String())
	}
}

func TestResumeCorruptStateIsolated(t *testing.T) {
	cfg := testConfig(t)
	s, _ := newTestSession(t, cfg, &mockEngine{})

	if err := os.WriteFile(cfg.SessionStatePath, []byte("{broken"), 0644); err != nil {
		t.Fatalf("gagal menulis state rusak: %v", err)
	}
	s.HandleCommand("/resume")

	if _, err := os.Stat(cfg.SessionStatePath + ".broken"); err != nil {
		t.Errorf("state rusak harus dipindah ke .broken, err=%v", err)
	}
	if _, err := os.Stat(cfg.SessionStatePath); !os.IsNotExist(err) {
		t.Errorf("state rusak tidak boleh tetap di path asli, err=%v", err)
	}
}

func TestNewCommandKeepsCustomPersona(t *testing.T) {
	s, _ := newTestSession(t, testConfig(t), &mockEngine{})

	s.HandleCommand("/roleplay kamu adalah sysadmin 1964")
	s.HandleCommand("/new")

	if prompt := s.BuildSystemPrompt(); !strings.Contains(prompt, "kamu adalah sysadmin 1964") {
		t.Errorf("/new harus mempertahankan persona kustom, dapat:\n%s", prompt)
	}
}

func TestSaveStatePersistsApprovedPlan(t *testing.T) {
	cfg := testConfig(t)
	s, _ := newTestSession(t, cfg, &mockEngine{})
	s.macroMem.SetApprovedPlan("RENCANA_DISETUJUI")

	if err := s.saveState(); err != nil {
		t.Fatalf("saveState gagal: %v", err)
	}
	data, err := os.ReadFile(cfg.SessionStatePath)
	if err != nil {
		t.Fatalf("gagal membaca state: %v", err)
	}
	if !strings.Contains(string(data), "RENCANA_DISETUJUI") {
		t.Errorf("state harus menyimpan approved plan, dapat: %s", data)
	}
}

func TestWorkerExhaustionReturnsError(t *testing.T) {
	eng := &mockEngine{}
	eng.handler = func(_ int, _ engineCall) (*types.AgentResponse, error) {
		return &types.AgentResponse{
			Thought: "Saya membaca berkas yang tidak ada lagi.",
			Action:  types.ActionRead,
			Input:   "tidak-ada.txt",
		}, nil
	}
	s, _ := newTestSession(t, testConfig(t), eng)

	if _, err := s.RunWorker(context.Background(), "tugas tak selesai"); err == nil {
		t.Error("pekerja yang kehabisan langkah harus mengembalikan error")
	}
}

func TestRunTurnPausesOrchestratorForWorker(t *testing.T) {
	cfg := testConfig(t)
	cfg.Security.DefaultMode = types.ModeAuto
	logPath := filepath.Join(cfg.WorkspaceDir, "catatan.txt")
	if err := os.WriteFile(logPath, []byte("ISI_LOGFILE"), 0644); err != nil {
		t.Fatalf("gagal menyiapkan file workspace: %v", err)
	}

	eng := &mockEngine{}
	eng.handler = func(idx int, call engineCall) (*types.AgentResponse, error) {
		if containsAction(call.allowed, types.ActionDelegate) {
			if strings.Contains(messagesText(call.messages), "LAPORAN_PEKERJA") {
				return &types.AgentResponse{
					Thought: "Pekerja sudah melapor, saya akhiri giliran.",
					Action:  types.ActionFinalAnswer,
					Input:   "SELESAI",
				}, nil
			}
			return &types.AgentResponse{
				Thought: "Saya delegasikan pemeriksaan catatan.",
				Action:  types.ActionDelegate,
				Input:   "baca catatan.txt",
			}, nil
		}

		if !strings.Contains(messagesText(call.messages), "ISI_LOGFILE") {
			return &types.AgentResponse{
				Thought: "Saya baca file catatan terlebih dahulu.",
				Action:  types.ActionRead,
				Input:   "catatan.txt",
			}, nil
		}
		return &types.AgentResponse{
			Thought: "File sudah terbaca, saya laporkan hasilnya.",
			Action:  types.ActionReportDone,
			Input:   "LAPORAN_PEKERJA: isi catatan log",
		}, nil
	}

	s, _ := newTestSession(t, cfg, eng)
	s.HandleCommand("/work")

	result, err := s.RunTurn(context.Background(), "tolong cek catatan")
	if err != nil {
		t.Fatalf("RunTurn gagal: %v", err)
	}
	if result != "SELESAI" {
		t.Errorf("hasil RunTurn = %q, want SELESAI", result)
	}
	if got := eng.CallCount(); got != 4 {
		t.Fatalf("urutan panggilan harus orkestrator->pekerja(read)->pekerja(report)->orkestrator (4), dapat %d", got)
	}

	orch1 := eng.Call(0)
	if !containsAction(orch1.allowed, types.ActionDelegate) || !containsAction(orch1.allowed, types.ActionFinalAnswer) {
		t.Errorf("allowedActions orkestrator salah: %v", orch1.allowed)
	}

	worker1 := eng.Call(1)
	if !containsAction(worker1.allowed, types.ActionRead) || !containsAction(worker1.allowed, types.ActionReportDone) {
		t.Errorf("allowedActions pekerja salah: %v", worker1.allowed)
	}
	workerText := messagesText(worker1.messages)
	if !strings.Contains(workerText, "WORKER_SYSTEM_PROMPT") {
		t.Error("pekerja harus memakai worker system prompt")
	}
	for _, forbidden := range []string{"DEFAULT_ROLE_PROMPT", "DEFAULT_FEWSHOT", "WORK_MODE_PROMPT"} {
		if strings.Contains(workerText, forbidden) {
			t.Errorf("WorkerMemory tidak boleh memuat konteks orkestrator %q: %s", forbidden, workerText)
		}
	}
	if !strings.Contains(workerText, "baca catatan.txt") {
		t.Error("pekerja harus menerima instruksi tugas dari orkestrator")
	}
	if !strings.Contains(messagesText(eng.Call(2).messages), "ISI_LOGFILE") {
		t.Error("observasi alat harus masuk kembali ke WorkerMemory")
	}

	orch2 := eng.Call(3)
	orchText := messagesText(orch2.messages)
	if !strings.Contains(orchText, "LAPORAN_PEKERJA") {
		t.Errorf("laporan pekerja harus diteruskan sebagai observasi ke orkestrator: %s", orchText)
	}
}

func TestWorkerMemoryIsFreshAndNotRetained(t *testing.T) {
	eng := &mockEngine{}
	eng.handler = func(idx int, call engineCall) (*types.AgentResponse, error) {
		return &types.AgentResponse{
			Thought: "Tugas selesai, saya laporkan hasilnya.",
			Action:  types.ActionReportDone,
			Input:   "LAPORAN_UNIK",
		}, nil
	}
	s, _ := newTestSession(t, testConfig(t), eng)

	if _, err := s.RunWorker(context.Background(), "tugas pertama"); err != nil {
		t.Fatalf("RunWorker pertama gagal: %v", err)
	}
	if _, err := s.RunWorker(context.Background(), "tugas kedua"); err != nil {
		t.Fatalf("RunWorker kedua gagal: %v", err)
	}

	second := messagesText(eng.Call(1).messages)
	if strings.Contains(second, "tugas pertama") {
		t.Errorf("WorkerMemory harus dibuat segar tiap delegasi, bocor tugas lama: %s", second)
	}
	if !strings.Contains(second, "tugas kedua") {
		t.Errorf("WorkerMemory kedua harus memuat tugas barunya: %s", second)
	}

	typ := reflect.TypeOf(Session{})
	for i := 0; i < typ.NumField(); i++ {
		field := strings.ToLower(typ.Field(i).Name)
		if strings.Contains(field, "worker") {
			t.Errorf("Session tidak boleh menyimpan WorkerMemory agar langsung di-GC, field: %q", typ.Field(i).Name)
		}
	}
}

func containsAction(actions []string, target string) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}

func messagesText(messages []types.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(msg.Content)
		b.WriteString("\n")
	}
	return b.String()
}
