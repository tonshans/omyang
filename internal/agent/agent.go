// Package agent owns the orchestrator/worker turn loop. Session only supplies
// the runtime state and progress callback.
package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"omyang/internal/config"
	"omyang/internal/engine"
	"omyang/internal/memory"
	"omyang/internal/sanitizer"
	"omyang/internal/tools"
	"omyang/internal/types"
)

// Runner executes one user turn against an engine, its macro-memory, and tools.
type Runner struct {
	Engine      engine.Engine
	Memory      *memory.OrchestratorMemory
	Tools       *tools.Executor
	Config      *config.Config
	SessionMode string
	OnProgress  func()
}

// RunTurn drives the orchestrator until final_answer, pausing synchronously for
// RunWorker on every delegate_task.
func (r *Runner) RunTurn(ctx context.Context, input string) (string, error) {
	allowed := types.OrchestratorActions(r.SessionMode)
	monitor := engine.NewCircuitBreakerMonitor(r.Config.CircuitBreaker)

	for step := 1; ; step++ {
		resp, err := r.Engine.Generate(ctx, r.Memory.BuildMessages(r.SessionMode, input), allowed)
		if err != nil {
			return "", err
		}
		if err := monitor.CheckOrchestratorStep(step, resp); err != nil {
			return "", err
		}

		if resp.Action == types.ActionDelegate {
			report, err := r.RunWorker(ctx, resp.Input)
			if err != nil {
				return "", err
			}
			r.Memory.AddPair(types.MessagePair{
				UserMsg:      types.Message{Role: "user", Content: resp.Input},
				AssistantMsg: types.Message{Role: "assistant", Content: report},
			})
			r.progress()
			continue
		}

		r.Memory.AddPair(types.MessagePair{
			UserMsg:      types.Message{Role: "user", Content: input},
			AssistantMsg: types.Message{Role: "assistant", Content: resp.Input},
		})
		r.progress()
		return resp.Input, nil
	}
}

// RunWorker executes one sterile worker task. WorkerMemory is a local value and
// is discarded as soon as the function returns.
func (r *Runner) RunWorker(ctx context.Context, taskInstruction string) (string, error) {
	workerMem := memory.NewWorkerMemory(r.Config.WorkerPrompt, taskInstruction)
	dedup := &sanitizer.ErrorDeduplicator{}
	allowed := types.WorkerActions()

	maxSteps := r.Config.CircuitBreaker.WorkerMaxSteps
	if maxSteps <= 0 {
		maxSteps = types.DefaultWorkerMaxSteps
	}
	headLen := r.Config.Memory.TruncationHeadChars
	if headLen <= 0 {
		headLen = types.DefaultOutputHeadChars
	}
	tailLen := r.Config.Memory.TruncationTailChars
	if tailLen <= 0 {
		tailLen = types.DefaultOutputTailChars
	}

	for step := 0; step < maxSteps; step++ {
		resp, err := r.Engine.Generate(ctx, workerMem.BuildMessages(), allowed)
		if err != nil {
			return "", err
		}
		if resp.Action == types.ActionReportDone {
			return resp.Input, nil
		}

		output, execErr := r.Tools.Execute(resp.Action, resp.Input)
		observation := ""
		if execErr != nil {
			observation = execErr.Error()
		} else {
			observation = sanitizer.CleanToolOutput(output, headLen, tailLen)
		}
		observation = dedup.Process(observation)

		assistantJSON, err := json.Marshal(resp)
		if err != nil {
			assistantJSON = []byte(fmt.Sprintf(`{"thought":%q,"action":%q,"input":%q}`, resp.Thought, resp.Action, resp.Input))
		}
		workerMem.AddStep(string(assistantJSON), observation)
	}

	return "", fmt.Errorf("pekerja gagal mencapai report_done dalam %d langkah", maxSteps)
}

func (r *Runner) progress() {
	if r.OnProgress != nil {
		r.OnProgress()
	}
}
