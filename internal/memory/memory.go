package memory

import (
	"strings"

	"omyang/internal/config"
	"omyang/internal/types"
)

const compressionSummaryMarker = "[Ringkasan Konteks Sebelumnya]"

// OrchestratorMemory is the Macro-Memory of the supervisor agent.
type OrchestratorMemory struct {
	pairs                []types.MessagePair
	maxPairs             int
	compressionThreshold int
	modePrompts          config.ModePrompts
	roleProfile          types.RoleProfile
	customRolePrompt     string
	activeSkillContent   string
	approvedPlan         string
}

// NewOrchestratorMemory creates a Macro-Memory with its sliding-window and compression limits.
func NewOrchestratorMemory(maxPairs int, compressionThreshold int, modePrompts config.ModePrompts, roleProfile types.RoleProfile) *OrchestratorMemory {
	return &OrchestratorMemory{
		maxPairs:             maxPairs,
		compressionThreshold: compressionThreshold,
		modePrompts:          modePrompts,
		roleProfile:          roleProfile,
	}
}

func (m *OrchestratorMemory) SetRoleProfile(roleProfile types.RoleProfile) {
	m.roleProfile = roleProfile
}

func (m *OrchestratorMemory) SetCustomRolePrompt(prompt string) {
	m.customRolePrompt = prompt
}

func (m *OrchestratorMemory) SetSkillContent(content string) {
	m.activeSkillContent = content
}

func (m *OrchestratorMemory) SetApprovedPlan(plan string) {
	m.approvedPlan = plan
}

// RoleProfile returns the active role profile.
func (m *OrchestratorMemory) RoleProfile() types.RoleProfile {
	return m.roleProfile
}

// CustomRolePrompt returns the active /roleplay persona, if any.
func (m *OrchestratorMemory) CustomRolePrompt() string {
	return m.customRolePrompt
}

// SkillContent returns the active skill content, if any.
func (m *OrchestratorMemory) SkillContent() string {
	return m.activeSkillContent
}

// ApprovedPlan returns the approved /work plan, if any.
func (m *OrchestratorMemory) ApprovedPlan() string {
	return m.approvedPlan
}

// Pairs returns a copy of the current history pairs.
func (m *OrchestratorMemory) Pairs() []types.MessagePair {
	return append([]types.MessagePair(nil), m.pairs...)
}

// AddPair appends a pair and drops the oldest pairs beyond maxPairs.
func (m *OrchestratorMemory) AddPair(pair types.MessagePair) {
	m.pairs = append(m.pairs, pair)
	if m.maxPairs > 0 && len(m.pairs) > m.maxPairs {
		m.pairs = append([]types.MessagePair(nil), m.pairs[len(m.pairs)-m.maxPairs:]...)
	}
}

// BuildSystemPrompt assembles mode, role, skill, approved plan, and JSON schema rules in order.
func (m *OrchestratorMemory) BuildSystemPrompt(sessionMode string) string {
	var sections []string
	add := func(section string) {
		if strings.TrimSpace(section) != "" {
			sections = append(sections, section)
		}
	}

	switch sessionMode {
	case types.SessionPlan:
		add(m.modePrompts.Plan)
	case types.SessionWork:
		add(m.modePrompts.Work)
	default:
		add(m.modePrompts.Talk)
	}

	if m.customRolePrompt != "" {
		add(m.customRolePrompt)
	} else {
		add(m.roleProfile.SystemPrompt)
	}

	add(m.activeSkillContent)

	if sessionMode == types.SessionWork {
		add(m.approvedPlan)
	}

	add(schemaRules(sessionMode))
	return strings.Join(sections, "\n\n")
}

func schemaRules(sessionMode string) string {
	actions := types.OrchestratorActions(sessionMode)
	rules := "## Aturan Output JSON\nBalas HANYA dengan JSON valid: {\"thought\":\"...\",\"action\":\"" +
		strings.Join(actions, "|") + "\",\"input\":\"...\"}.\n"
	if sessionMode == types.SessionWork {
		return rules + "Gunakan delegate_task untuk mendelegasikan instruksi teknis kepada Pekerja, atau final_answer untuk mengakhiri giliran."
	}
	return rules + "Hanya aksi final_answer yang tersedia dalam mode ini."
}

// BuildMessages returns [SystemPrompt, FewShot..., HistoryPairs..., CurrentUserMessage].
func (m *OrchestratorMemory) BuildMessages(sessionMode string, currentUserContent string) []types.Message {
	messages := make([]types.Message, 0, 2+len(m.roleProfile.FewShot)+len(m.pairs)*2)
	messages = append(messages, types.Message{Role: "system", Content: m.BuildSystemPrompt(sessionMode)})
	messages = append(messages, m.roleProfile.FewShot...)
	for _, pair := range m.pairs {
		messages = append(messages, pair.UserMsg, pair.AssistantMsg)
	}
	messages = append(messages, types.Message{Role: "user", Content: currentUserContent})
	return messages
}

// NeedsCompression reports whether history pairs reached the compression threshold.
// shortcut: not wired into the turn loop yet (history is only trimmed by maxPairs),
// wire into agent.Runner when session summarization is implemented.
func (m *OrchestratorMemory) NeedsCompression() bool {
	return len(m.pairs) >= m.compressionThreshold
}

// ApplyCompression replaces old pairs with one summary pair while keeping the newest keepRecent pairs.
func (m *OrchestratorMemory) ApplyCompression(summaryText string, keepRecent int) {
	if keepRecent < 0 {
		keepRecent = 0
	}
	if len(m.pairs) <= keepRecent {
		return
	}

	recent := append([]types.MessagePair(nil), m.pairs[len(m.pairs)-keepRecent:]...)
	summary := types.MessagePair{
		UserMsg:      types.Message{Role: "user", Content: summaryText},
		AssistantMsg: types.Message{Role: "assistant", Content: compressionSummaryMarker},
	}
	m.pairs = append([]types.MessagePair{summary}, recent...)
}

type workerStep struct {
	assistantJSON string
	observation   string
}

// WorkerMemory is the sterile Micro-Memory. It never holds a Role or Skill.
type WorkerMemory struct {
	systemPrompt    string
	fewShot         []types.Message
	taskInstruction string
	steps           []workerStep
}

// NewWorkerMemory creates a fresh Micro-Memory from the worker config and one task instruction.
func NewWorkerMemory(workerCfg config.WorkerPromptCfg, taskInstruction string) *WorkerMemory {
	return &WorkerMemory{
		systemPrompt:    workerCfg.SystemPrompt,
		fewShot:         workerCfg.FewShot,
		taskInstruction: taskInstruction,
	}
}

// AddStep records one assistant action and its tool observation.
func (w *WorkerMemory) AddStep(assistantJSON string, observation string) {
	w.steps = append(w.steps, workerStep{assistantJSON: assistantJSON, observation: observation})
}

// BuildMessages returns the worker conversation, including recorded steps.
func (w *WorkerMemory) BuildMessages() []types.Message {
	messages := make([]types.Message, 0, 2+len(w.fewShot)+len(w.steps)*2)
	messages = append(messages, types.Message{Role: "system", Content: w.systemPrompt})
	messages = append(messages, w.fewShot...)
	messages = append(messages, types.Message{Role: "user", Content: w.taskInstruction})
	for _, step := range w.steps {
		messages = append(messages,
			types.Message{Role: "assistant", Content: step.assistantJSON},
			types.Message{Role: "user", Content: step.observation},
		)
	}
	return messages
}
