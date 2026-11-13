package types

// Konstanta EngineType
const (
	EngineLocal = "local"
	EngineCloud = "cloud"
)

// Konstanta SecurityMode
const (
	ModeCopilot = "copilot"
	ModeAuto    = "auto"
)

// Konstanta SessionMode (State Machine)
const (
	SessionTalk = "talk"
	SessionPlan = "plan"
	SessionWork = "work"
)

// Konstanta Action Orkestrator (Macro-Agent)
const (
	ActionDelegate    = "delegate_task"
	ActionFinalAnswer = "final_answer"
)

// Konstanta Action Pekerja (Micro-Agent)
const (
	ActionRead       = "read"
	ActionWrite      = "write"
	ActionBash       = "bash"
	ActionReportDone = "report_done"
)

// Konstanta Nilai Default & Batas Sistem
const (
	DefaultMaxSkillChars          = 4000
	DefaultOutputHeadChars        = 800
	DefaultOutputTailChars        = 1200
	DefaultOrchMaxHistoryPairs    = 6
	DefaultMacroCompressionThresh = 5
	DefaultOrchMaxSteps           = 4
	DefaultWorkerMaxSteps         = 3
	DefaultMinThoughtLength       = 15
	DefaultMaxDuplicateDelegation = 2
	DefaultTimeoutSeconds         = 30
	DummyGeminiAPIKey             = "YOUR_GEMINI_API_KEY_HERE"
	AutoModelPath                 = "auto"
	CustomRoleName                = "custom*"
	DefaultRoleName               = "default"
)

// OrchestratorActions adalah daftar aksi yang diizinkan untuk giliran Orkestrator.
// Satu-satunya sumber kebenaran untuk prompt, skema JSON, dan validasi engine.
func OrchestratorActions(sessionMode string) []string {
	if sessionMode == SessionWork {
		return []string{ActionDelegate, ActionFinalAnswer}
	}
	return []string{ActionFinalAnswer}
}

// WorkerActions adalah daftar aksi alat yang tersedia bagi Pekerja.
func WorkerActions() []string {
	return []string{ActionRead, ActionWrite, ActionBash, ActionReportDone}
}

// AgentResponse adalah kontrak JSON ketat yang dihasilkan oleh LLM (Orchestrator maupun Worker).
type AgentResponse struct {
	Thought string `json:"thought"`
	Action  string `json:"action"`
	Input   string `json:"input"`
}

// Message merepresentasikan satu pesan dalam riwayat percakapan LLM.
type Message struct {
	Role    string `json:"role"`    // "system", "user", atau "assistant"
	Content string `json:"content"` // Teks pesan atau string JSON dari AgentResponse
}

// MessagePair merepresentasikan sepasang interaksi utuh agar tidak ada konteks yatim-piatu.
type MessagePair struct {
	UserMsg      Message `json:"user_msg"`
	AssistantMsg Message `json:"assistant_msg"`
}

// RoleProfile mendefinisikan satu persona Orkestrator di dalam roles.json.
type RoleProfile struct {
	SystemPrompt string    `json:"system_prompt"`
	FewShot      []Message `json:"few_shot"`
}

// SessionState merepresentasikan struktur file ./.session_state.json untuk fitur /resume.
type SessionState struct {
	ActiveEngine string        `json:"active_engine"`
	SecurityMode string        `json:"security_mode"`
	SessionMode  string        `json:"session_mode"`
	ActiveRole   string        `json:"active_role"`
	CustomPrompt string        `json:"custom_prompt,omitempty"`
	ActiveSkill  string        `json:"active_skill"`
	ApprovedPlan string        `json:"approved_plan,omitempty"`
	Pairs        []MessagePair `json:"pairs"`
}
