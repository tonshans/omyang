package memory

import (
	"reflect"
	"strings"
	"testing"

	"omyang/internal/config"
	"omyang/internal/types"
)

func newTestOrchestratorMemory(maxPairs, compressionThreshold int) *OrchestratorMemory {
	return NewOrchestratorMemory(
		maxPairs,
		compressionThreshold,
		config.ModePrompts{
			Talk: "TALK_MODE_PROMPT",
			Plan: "PLAN_MODE_PROMPT",
			Work: "WORK_MODE_PROMPT",
		},
		types.RoleProfile{
			SystemPrompt: "BASE_ROLE_PROMPT",
			FewShot: []types.Message{
				{Role: "user", Content: "ROLE_FEWSHOT"},
			},
		},
	)
}

func testPair(id string) types.MessagePair {
	return types.MessagePair{
		UserMsg:      types.Message{Role: "user", Content: "USER_" + id},
		AssistantMsg: types.Message{Role: "assistant", Content: "ASSISTANT_" + id},
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected prompt to contain %q, got:\n%s", needle, haystack)
	}
}

func assertNotContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Errorf("expected prompt NOT to contain %q, got:\n%s", needle, haystack)
	}
}

func assertOrdered(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	last := -1
	for _, needle := range needles {
		idx := strings.Index(haystack, needle)
		if idx == -1 {
			t.Fatalf("expected prompt to contain %q, got:\n%s", needle, haystack)
		}
		if idx <= last {
			t.Errorf("expected %q after previous section %q in prompt:\n%s", needle, haystack, haystack)
		}
		last = idx
	}
}

func TestBuildSystemPromptWork(t *testing.T) {
	mem := newTestOrchestratorMemory(6, 5)
	mem.SetSkillContent("SKILL_CONTENT")
	mem.SetApprovedPlan("APPROVED_PLAN")

	prompt := mem.BuildSystemPrompt(types.SessionWork)

	assertOrdered(t, prompt, "WORK_MODE_PROMPT", "BASE_ROLE_PROMPT", "SKILL_CONTENT", "APPROVED_PLAN", "delegate_task")
	assertContains(t, prompt, types.ActionDelegate)
	assertContains(t, prompt, types.ActionFinalAnswer)
}

func TestBuildSystemPromptPlanAndTalk(t *testing.T) {
	for _, sessionMode := range []string{types.SessionPlan, types.SessionTalk} {
		t.Run(sessionMode, func(t *testing.T) {
			mem := newTestOrchestratorMemory(6, 5)
			mem.SetSkillContent("SKILL_CONTENT")
			mem.SetApprovedPlan("APPROVED_PLAN")

			prompt := mem.BuildSystemPrompt(sessionMode)

			modePrompt := "PLAN_MODE_PROMPT"
			if sessionMode == types.SessionTalk {
				modePrompt = "TALK_MODE_PROMPT"
			}
			assertOrdered(t, prompt, modePrompt, "BASE_ROLE_PROMPT", "SKILL_CONTENT", types.ActionFinalAnswer)
			assertNotContains(t, prompt, "APPROVED_PLAN")
			assertNotContains(t, prompt, types.ActionDelegate)
		})
	}
}

func TestBuildSystemPromptCustomRoleOverridesProfile(t *testing.T) {
	mem := newTestOrchestratorMemory(6, 5)
	mem.SetCustomRolePrompt("CUSTOM_ROLE_PROMPT")

	prompt := mem.BuildSystemPrompt(types.SessionTalk)

	assertContains(t, prompt, "CUSTOM_ROLE_PROMPT")
	assertNotContains(t, prompt, "BASE_ROLE_PROMPT")
}

func TestBuildMessagesOrder(t *testing.T) {
	mem := newTestOrchestratorMemory(6, 5)
	mem.AddPair(testPair("ONE"))
	mem.AddPair(testPair("TWO"))

	messages := mem.BuildMessages(types.SessionWork, "CURRENT_USER")

	// system + 1 few-shot + 2 pairs (4 messages) + current user
	if len(messages) != 7 {
		t.Fatalf("expected 7 messages, got %d: %+v", len(messages), messages)
	}
	if messages[0].Role != "system" || messages[0].Content != mem.BuildSystemPrompt(types.SessionWork) {
		t.Errorf("expected first message to be the system prompt, got %+v", messages[0])
	}
	if messages[1].Content != "ROLE_FEWSHOT" {
		t.Errorf("expected role few-shot at index 1, got %+v", messages[1])
	}
	expected := []string{"USER_ONE", "ASSISTANT_ONE", "USER_TWO", "ASSISTANT_TWO", "CURRENT_USER"}
	for i, content := range expected {
		got := messages[i+2]
		if got.Content != content {
			t.Errorf("expected message %d content %q, got %q", i+2, content, got.Content)
		}
	}
	if messages[6].Role != "user" {
		t.Errorf("expected current message role user, got %q", messages[6].Role)
	}
}

func TestPairedSlidingWindowDropsOldestPair(t *testing.T) {
	mem := newTestOrchestratorMemory(2, 5)
	mem.AddPair(testPair("ONE"))
	mem.AddPair(testPair("TWO"))
	mem.AddPair(testPair("THREE"))

	if len(mem.pairs) != 2 {
		t.Fatalf("expected 2 pairs, got %d", len(mem.pairs))
	}
	if mem.pairs[0].UserMsg.Content != "USER_TWO" || mem.pairs[1].UserMsg.Content != "USER_THREE" {
		t.Errorf("expected oldest pair dropped whole, got %+v", mem.pairs)
	}

	messages := mem.BuildMessages(types.SessionWork, "CURRENT_USER")
	for _, msg := range messages {
		if strings.Contains(msg.Content, "USER_ONE") || strings.Contains(msg.Content, "ASSISTANT_ONE") {
			t.Errorf("dropped pair leaked into messages: %+v", msg)
		}
	}
}

func TestNeedsCompression(t *testing.T) {
	mem := newTestOrchestratorMemory(10, 3)
	mem.AddPair(testPair("ONE"))
	mem.AddPair(testPair("TWO"))
	if mem.NeedsCompression() {
		t.Error("expected no compression below threshold")
	}
	mem.AddPair(testPair("THREE"))
	if !mem.NeedsCompression() {
		t.Error("expected compression at threshold")
	}
}

func TestApplyCompression(t *testing.T) {
	mem := newTestOrchestratorMemory(10, 3)
	for _, id := range []string{"ONE", "TWO", "THREE", "FOUR", "FIVE"} {
		mem.AddPair(testPair(id))
	}

	mem.ApplyCompression("SUMMARY_TEXT", 2)

	if len(mem.pairs) != 3 {
		t.Fatalf("expected summary pair plus 2 recent pairs, got %d", len(mem.pairs))
	}
	if mem.pairs[0].UserMsg.Content != "SUMMARY_TEXT" {
		t.Errorf("expected summary text in first user message, got %q", mem.pairs[0].UserMsg.Content)
	}
	if mem.pairs[0].AssistantMsg.Content != "[Ringkasan Konteks Sebelumnya]" {
		t.Errorf("expected summary marker, got %q", mem.pairs[0].AssistantMsg.Content)
	}
	if mem.pairs[1].UserMsg.Content != "USER_FOUR" || mem.pairs[2].UserMsg.Content != "USER_FIVE" {
		t.Errorf("expected recent pairs preserved, got %+v", mem.pairs)
	}
}

func TestApplyCompressionKeepsAllWhenHistoryIsShort(t *testing.T) {
	mem := newTestOrchestratorMemory(10, 3)
	mem.AddPair(testPair("ONE"))

	mem.ApplyCompression("SUMMARY_TEXT", 5)

	if len(mem.pairs) != 1 || mem.pairs[0].UserMsg.Content != "USER_ONE" {
		t.Errorf("expected short history unchanged, got %+v", mem.pairs)
	}
}

func TestWorkerMemoryIsSterileAndOrdered(t *testing.T) {
	orchestrator := newTestOrchestratorMemory(6, 5)
	orchestrator.SetSkillContent("SECRET_SKILL_CONTENT")
	orchestrator.SetCustomRolePrompt("SECRET_ROLE_PROMPT")

	workerCfg := config.WorkerPromptCfg{
		SystemPrompt: "WORKER_SYSTEM_PROMPT",
		FewShot: []types.Message{
			{Role: "user", Content: "WORKER_FEWSHOT"},
		},
	}
	worker := NewWorkerMemory(workerCfg, "TASK_INSTRUCTION")
	worker.AddStep(`{"thought":"run","action":"bash","input":"ls"}`, "OBSERVATION_ONE")

	messages := worker.BuildMessages()

	if len(messages) != 5 {
		t.Fatalf("expected 5 messages (system, few-shot, task, assistant, observation), got %d: %+v", len(messages), messages)
	}
	if messages[0].Content != "WORKER_SYSTEM_PROMPT" {
		t.Errorf("expected worker system prompt, got %q", messages[0].Content)
	}
	if messages[1].Content != "WORKER_FEWSHOT" {
		t.Errorf("expected worker few-shot, got %q", messages[1].Content)
	}
	if messages[2].Content != "TASK_INSTRUCTION" {
		t.Errorf("expected task instruction, got %q", messages[2].Content)
	}
	if messages[3].Content != `{"thought":"run","action":"bash","input":"ls"}` || messages[3].Role != "assistant" {
		t.Errorf("expected assistant step, got %+v", messages[3])
	}
	if messages[4].Content != "OBSERVATION_ONE" || messages[4].Role != "user" {
		t.Errorf("expected observation step, got %+v", messages[4])
	}

	var joined strings.Builder
	for _, msg := range messages {
		joined.WriteString(msg.Content)
		joined.WriteString("\n")
	}
	allContent := joined.String()
	for _, forbidden := range []string{"SECRET_ROLE_PROMPT", "SECRET_SKILL_CONTENT", "BASE_ROLE_PROMPT", "ROLE_FEWSHOT"} {
		if strings.Contains(allContent, forbidden) {
			t.Errorf("worker memory leaked orchestrator context %q: %s", forbidden, allContent)
		}
	}

	typ := reflect.TypeOf(WorkerMemory{})
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		if strings.Contains(name, "role") || strings.Contains(name, "skill") {
			t.Errorf("WorkerMemory must not have Role/Skill field, found %q", typ.Field(i).Name)
		}
	}
}
