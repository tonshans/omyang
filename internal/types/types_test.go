package types_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"omyang/internal/types"
)

func TestConstants(t *testing.T) {
	// EngineType
	if types.EngineLocal != "local" {
		t.Errorf("expected EngineLocal to be 'local', got '%s'", types.EngineLocal)
	}
	if types.EngineCloud != "cloud" {
		t.Errorf("expected EngineCloud to be 'cloud', got '%s'", types.EngineCloud)
	}

	// SecurityMode
	if types.ModeCopilot != "copilot" {
		t.Errorf("expected ModeCopilot to be 'copilot', got '%s'", types.ModeCopilot)
	}
	if types.ModeAuto != "auto" {
		t.Errorf("expected ModeAuto to be 'auto', got '%s'", types.ModeAuto)
	}

	// SessionMode
	if types.SessionTalk != "talk" {
		t.Errorf("expected SessionTalk to be 'talk', got '%s'", types.SessionTalk)
	}
	if types.SessionPlan != "plan" {
		t.Errorf("expected SessionPlan to be 'plan', got '%s'", types.SessionPlan)
	}
	if types.SessionWork != "work" {
		t.Errorf("expected SessionWork to be 'work', got '%s'", types.SessionWork)
	}

	// Action Orchestrator
	if types.ActionDelegate != "delegate_task" {
		t.Errorf("expected ActionDelegate to be 'delegate_task', got '%s'", types.ActionDelegate)
	}
	if types.ActionFinalAnswer != "final_answer" {
		t.Errorf("expected ActionFinalAnswer to be 'final_answer', got '%s'", types.ActionFinalAnswer)
	}

	// Action Worker
	if types.ActionRead != "read" {
		t.Errorf("expected ActionRead to be 'read', got '%s'", types.ActionRead)
	}
	if types.ActionWrite != "write" {
		t.Errorf("expected ActionWrite to be 'write', got '%s'", types.ActionWrite)
	}
	if types.ActionBash != "bash" {
		t.Errorf("expected ActionBash to be 'bash', got '%s'", types.ActionBash)
	}
	if types.ActionReportDone != "report_done" {
		t.Errorf("expected ActionReportDone to be 'report_done', got '%s'", types.ActionReportDone)
	}

	// System Defaults & Limits
	if types.DefaultMaxSkillChars != 4000 {
		t.Errorf("expected DefaultMaxSkillChars to be 4000, got %d", types.DefaultMaxSkillChars)
	}
	if types.DefaultOutputHeadChars != 800 {
		t.Errorf("expected DefaultOutputHeadChars to be 800, got %d", types.DefaultOutputHeadChars)
	}
	if types.DefaultOutputTailChars != 1200 {
		t.Errorf("expected DefaultOutputTailChars to be 1200, got %d", types.DefaultOutputTailChars)
	}
	if types.DefaultOrchMaxHistoryPairs != 6 {
		t.Errorf("expected DefaultOrchMaxHistoryPairs to be 6, got %d", types.DefaultOrchMaxHistoryPairs)
	}
	if types.DefaultMacroCompressionThresh != 5 {
		t.Errorf("expected DefaultMacroCompressionThresh to be 5, got %d", types.DefaultMacroCompressionThresh)
	}
	if types.DefaultOrchMaxSteps != 4 {
		t.Errorf("expected DefaultOrchMaxSteps to be 4, got %d", types.DefaultOrchMaxSteps)
	}
	if types.DefaultWorkerMaxSteps != 3 {
		t.Errorf("expected DefaultWorkerMaxSteps to be 3, got %d", types.DefaultWorkerMaxSteps)
	}
	if types.DefaultMinThoughtLength != 15 {
		t.Errorf("expected DefaultMinThoughtLength to be 15, got %d", types.DefaultMinThoughtLength)
	}
	if types.DefaultMaxDuplicateDelegation != 2 {
		t.Errorf("expected DefaultMaxDuplicateDelegation to be 2, got %d", types.DefaultMaxDuplicateDelegation)
	}
	if types.DefaultTimeoutSeconds != 30 {
		t.Errorf("expected DefaultTimeoutSeconds to be 30, got %d", types.DefaultTimeoutSeconds)
	}
	if types.DummyGeminiAPIKey != "YOUR_GEMINI_API_KEY_HERE" {
		t.Errorf("expected DummyGeminiAPIKey to be 'YOUR_GEMINI_API_KEY_HERE', got '%s'", types.DummyGeminiAPIKey)
	}
	if types.AutoModelPath != "auto" {
		t.Errorf("expected AutoModelPath to be 'auto', got '%s'", types.AutoModelPath)
	}
	if types.CustomRoleName != "custom*" {
		t.Errorf("expected CustomRoleName to be 'custom*', got '%s'", types.CustomRoleName)
	}
	if types.DefaultRoleName != "default" {
		t.Errorf("expected DefaultRoleName to be 'default', got '%s'", types.DefaultRoleName)
	}
}

func TestAgentResponseSerialization(t *testing.T) {
	orig := types.AgentResponse{
		Thought: "Thinking about file reading",
		Action:  types.ActionRead,
		Input:   "README.md",
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"thought":"Thinking about file reading"`) {
		t.Errorf("expected json to contain thought, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"action":"read"`) {
		t.Errorf("expected json to contain action, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"input":"README.md"`) {
		t.Errorf("expected json to contain input, got %s", jsonStr)
	}

	var parsed types.AgentResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(orig, parsed) {
		t.Errorf("roundtrip mismatch: expected %+v, got %+v", orig, parsed)
	}
}

func TestMessageSerialization(t *testing.T) {
	orig := types.Message{
		Role:    "user",
		Content: "halo asisten",
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"role":"user"`) {
		t.Errorf("expected json to contain role, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"content":"halo asisten"`) {
		t.Errorf("expected json to contain content, got %s", jsonStr)
	}

	var parsed types.Message
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(orig, parsed) {
		t.Errorf("roundtrip mismatch: expected %+v, got %+v", orig, parsed)
	}
}

func TestMessagePairSerialization(t *testing.T) {
	orig := types.MessagePair{
		UserMsg: types.Message{
			Role:    "user",
			Content: "jalankan tes",
		},
		AssistantMsg: types.Message{
			Role:    "assistant",
			Content: `{"thought":"menjalankan tes","action":"bash","input":"go test ./..."}`,
		},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"user_msg"`) {
		t.Errorf("expected json to contain user_msg, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"assistant_msg"`) {
		t.Errorf("expected json to contain assistant_msg, got %s", jsonStr)
	}

	var parsed types.MessagePair
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(orig, parsed) {
		t.Errorf("roundtrip mismatch: expected %+v, got %+v", orig, parsed)
	}
}

func TestRoleProfileSerialization(t *testing.T) {
	orig := types.RoleProfile{
		SystemPrompt: "Kamu adalah asisten ahli termux",
		FewShot: []types.Message{
			{Role: "user", Content: "bantu saya"},
			{Role: "assistant", Content: `{"thought":"bantu user","action":"final_answer","input":"Siap!"}`},
		},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"system_prompt":"Kamu adalah asisten ahli termux"`) {
		t.Errorf("expected json to contain system_prompt, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"few_shot"`) {
		t.Errorf("expected json to contain few_shot, got %s", jsonStr)
	}

	var parsed types.RoleProfile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(orig, parsed) {
		t.Errorf("roundtrip mismatch: expected %+v, got %+v", orig, parsed)
	}
}

func TestSessionStateSerialization(t *testing.T) {
	t.Run("Full fields present", func(t *testing.T) {
		orig := types.SessionState{
			ActiveEngine: types.EngineLocal,
			SecurityMode: types.ModeCopilot,
			SessionMode:  types.SessionWork,
			ActiveRole:   types.CustomRoleName,
			CustomPrompt: "Custom system prompt persona",
			ActiveSkill:  "termux_api",
			ApprovedPlan: "1. Baca file\n2. Eksekusi",
			Pairs: []types.MessagePair{
				{
					UserMsg:      types.Message{Role: "user", Content: "buat rencana"},
					AssistantMsg: types.Message{Role: "assistant", Content: "rencana selesai"},
				},
			},
		}

		data, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		jsonStr := string(data)
		expectedKeys := []string{
			`"active_engine":"local"`,
			`"security_mode":"copilot"`,
			`"session_mode":"work"`,
			`"active_role":"custom*"`,
			`"custom_prompt":"Custom system prompt persona"`,
			`"active_skill":"termux_api"`,
			`"approved_plan":"1. Baca file\n2. Eksekusi"`,
			`"pairs"`,
		}
		for _, key := range expectedKeys {
			if !strings.Contains(jsonStr, key) {
				t.Errorf("expected json to contain %s, got %s", key, jsonStr)
			}
		}

		var parsed types.SessionState
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		if !reflect.DeepEqual(orig, parsed) {
			t.Errorf("roundtrip mismatch: expected %+v, got %+v", orig, parsed)
		}
	})

	t.Run("Omitempty fields omitted when empty", func(t *testing.T) {
		orig := types.SessionState{
			ActiveEngine: types.EngineCloud,
			SecurityMode: types.ModeAuto,
			SessionMode:  types.SessionTalk,
			ActiveRole:   types.DefaultRoleName,
			ActiveSkill:  "-",
			Pairs:        []types.MessagePair{},
		}

		data, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		jsonStr := string(data)
		if strings.Contains(jsonStr, `"custom_prompt"`) {
			t.Errorf("expected custom_prompt to be omitted, got %s", jsonStr)
		}
		if strings.Contains(jsonStr, `"approved_plan"`) {
			t.Errorf("expected approved_plan to be omitted, got %s", jsonStr)
		}

		var parsed types.SessionState
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		if parsed.CustomPrompt != "" || parsed.ApprovedPlan != "" {
			t.Errorf("expected empty optional fields, got %+v", parsed)
		}
	})
}

func TestOrchestratorAndWorkerActions(t *testing.T) {
	work := types.OrchestratorActions(types.SessionWork)
	if len(work) != 2 || work[0] != types.ActionDelegate || work[1] != types.ActionFinalAnswer {
		t.Errorf("work actions = %v, want [delegate_task final_answer]", work)
	}
	for _, mode := range []string{types.SessionTalk, types.SessionPlan} {
		actions := types.OrchestratorActions(mode)
		if len(actions) != 1 || actions[0] != types.ActionFinalAnswer {
			t.Errorf("%s actions = %v, want [final_answer]", mode, actions)
		}
	}
	if actions := types.WorkerActions(); len(actions) != 4 {
		t.Errorf("worker actions = %v, want 4", actions)
	}
}
