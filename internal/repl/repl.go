package repl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"omyang/internal/agent"
	"omyang/internal/config"
	"omyang/internal/engine"
	"omyang/internal/memory"
	"omyang/internal/skill"
	"omyang/internal/tools"
	"omyang/internal/types"
)

// Session holds all runtime state for one REPL session.
type Session struct {
	cfg *config.Config

	roles map[string]types.RoleProfile

	activeEngineType string
	sessionMode      string
	securityMode     string
	activeRoleName   string
	activeSkillName  string

	macroMem *memory.OrchestratorMemory
	toolExec *tools.Executor

	statelessMode bool

	eng           engine.Engine
	engineFactory func(string) (engine.Engine, error)
	in            *bufio.Reader
	out           io.Writer
}

// NewSession wires a Session from configuration, roles, and an inference engine.
func NewSession(
	cfg *config.Config,
	roles map[string]types.RoleProfile,
	eng engine.Engine,
	engineFactory func(string) (engine.Engine, error),
	in io.Reader,
	out io.Writer,
) *Session {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	if roles == nil {
		roles = map[string]types.RoleProfile{}
	}
	if _, ok := roles[types.DefaultRoleName]; !ok {
		roles[types.DefaultRoleName] = config.DefaultRoleProfile()
	}

	s := &Session{
		cfg:              cfg,
		roles:            roles,
		activeEngineType: cfg.Engine.ActiveEngine,
		sessionMode:      cfg.DefaultSession,
		activeRoleName:   cfg.DefaultRole,
		activeSkillName:  "-",
		eng:              eng,
		engineFactory:    engineFactory,
		in:               bufio.NewReader(in),
		out:              out,
	}
	if s.activeEngineType != types.EngineCloud {
		s.activeEngineType = types.EngineLocal
	}
	if s.sessionMode != types.SessionPlan && s.sessionMode != types.SessionWork {
		s.sessionMode = types.SessionTalk
	}
	s.securityMode = cfg.Security.DefaultMode
	if s.securityMode != types.ModeAuto {
		s.securityMode = types.ModeCopilot
	}
	if _, ok := roles[s.activeRoleName]; !ok {
		s.activeRoleName = types.DefaultRoleName
	}

	s.toolExec = &tools.Executor{
		WorkspaceDir:    cfg.WorkspaceDir,
		AllowedDirs:     cfg.Security.AllowedDirectories,
		BlockedPatterns: cfg.Security.BlockedCommandsRegex,
		SecurityMode:    s.securityMode,
		Timeout:         timeoutDuration(cfg),
		Reader:          s.in,
		Out:             out,
	}
	s.macroMem = s.newMacroMemory()
	return s
}

// SetStateless disables checkpoint persistence for one-off CLI sessions and
// removes interactive confirmation (no TTY is available there).
func (s *Session) SetStateless(stateless bool) {
	s.statelessMode = stateless
	if stateless {
		s.toolExec.Reader = nil
	}
}

// ReadLine reads one line from the interactive input stream.
func (s *Session) ReadLine() (string, error) {
	return s.in.ReadString('\n')
}

// Close releases the active engine.
func (s *Session) Close() error {
	if s.eng == nil {
		return nil
	}
	return s.eng.Close()
}

// FormatStatusBar renders the dynamic 1-row status bar.
func (s *Session) FormatStatusBar() string {
	engineLabel := "💻 Local"
	if s.activeEngineType == types.EngineCloud {
		engineLabel = "☁️ Cloud"
	}

	modeLabel := "💬 Talk"
	switch s.sessionMode {
	case types.SessionPlan:
		modeLabel = "🧠 Plan"
	case types.SessionWork:
		modeLabel = "🚀 Work"
	}

	parts := []string{engineLabel, modeLabel}
	// Security is only meaningful when terminal execution is possible (/work).
	if s.sessionMode == types.SessionWork {
		if s.securityMode == types.ModeAuto {
			parts = append(parts, "⚠️ Auto")
		} else {
			parts = append(parts, "🛡️ Copilot")
		}
	}
	parts = append(parts, "🎭 "+s.activeRoleName)

	skillLabel := "⚡ -"
	if s.activeSkillName != "" && s.activeSkillName != "-" {
		skillLabel = "⚡ " + s.activeSkillName
	}
	parts = append(parts, skillLabel)

	return "[ " + strings.Join(parts, " | ") + " ]"
}

// BuildSystemPrompt renders the orchestrator system prompt for the active mode.
func (s *Session) BuildSystemPrompt() string {
	return s.macroMem.BuildSystemPrompt(s.sessionMode)
}

// HandleCommand executes a slash command. It returns handled=false for normal prompts.
func (s *Session) HandleCommand(line string) (handled bool, exit bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "/") {
		return false, false
	}

	body := strings.TrimPrefix(trimmed, "/")
	name := body
	arg := ""
	if idx := strings.IndexAny(body, " \t"); idx >= 0 {
		name = body[:idx]
		arg = strings.TrimSpace(body[idx+1:])
	}

	switch name {
	case "talk":
		s.sessionMode = types.SessionTalk
	case "plan":
		s.sessionMode = types.SessionPlan
	case "work":
		s.sessionMode = types.SessionWork
	case "mode":
		s.handleModeCommand(arg)
	case "engine":
		s.handleEngineCommand(arg)
	case "role":
		s.handleRoleCommand(arg)
	case "roleplay":
		s.handleRoleplayCommand(arg)
	case "skill":
		s.handleSkillCommand(arg)
	case "new":
		s.handleNewCommand()
	case "resume":
		if err := s.Resume(); err != nil {
			fmt.Fprintf(s.out, "Gagal memulihkan sesi: %v\n", err)
		} else {
			fmt.Fprintln(s.out, "Sesi dipulihkan dari .session_state.json.")
		}
	case "btw":
		s.handleBtwCommand(arg)
	case "exit":
		return true, true
	default:
		fmt.Fprintf(s.out, "Perintah tidak dikenal: /%s\n", name)
	}
	return true, false
}

func (s *Session) handleModeCommand(arg string) {
	mode := strings.ToLower(strings.TrimSpace(arg))
	if mode != types.ModeAuto && mode != types.ModeCopilot {
		fmt.Fprintln(s.out, "Gunakan /mode auto atau /mode copilot.")
		return
	}
	s.securityMode = mode
	if s.toolExec != nil {
		s.toolExec.SecurityMode = mode
	}
	fmt.Fprintf(s.out, "Mode keamanan: %s\n", mode)
}

func (s *Session) handleEngineCommand(arg string) {
	target := strings.ToLower(strings.TrimSpace(arg))
	if target != types.EngineLocal && target != types.EngineCloud {
		fmt.Fprintln(s.out, "Gunakan /engine local atau /engine cloud.")
		return
	}
	if err := s.switchEngine(target); err != nil {
		fmt.Fprintf(s.out, "Gagal beralih engine: %v\n", err)
		return
	}
	fmt.Fprintf(s.out, "Engine aktif: %s\n", target)
}

func (s *Session) switchEngine(target string) error {
	if s.engineFactory != nil && target != s.activeEngineType {
		newEngine, err := s.engineFactory(target)
		if err != nil {
			return err
		}
		if s.eng != nil {
			_ = s.eng.Close()
		}
		s.eng = newEngine
	}
	s.activeEngineType = target
	return nil
}

func (s *Session) handleRoleCommand(arg string) {
	name := strings.TrimSpace(arg)
	if name == "" {
		names := make([]string, 0, len(s.roles))
		for roleName := range s.roles {
			names = append(names, roleName)
		}
		sort.Strings(names)
		fmt.Fprintf(s.out, "Role tersedia: %s\n", strings.Join(names, ", "))
		return
	}

	profile, ok := s.roles[name]
	if !ok {
		fmt.Fprintf(s.out, "Role %q tidak ditemukan.\n", name)
		return
	}
	s.activeRoleName = name
	s.macroMem.SetCustomRolePrompt("")
	s.macroMem.SetRoleProfile(profile)
	fmt.Fprintf(s.out, "Role aktif: %s\n", name)
}

func (s *Session) handleRoleplayCommand(arg string) {
	persona := strings.TrimSpace(arg)
	if persona == "" {
		fmt.Fprintln(s.out, "Gunakan /roleplay <deskripsi persona>.")
		return
	}
	s.activeRoleName = types.CustomRoleName
	// Custom role borrows the default few-shot so JSON formatting stays intact.
	s.macroMem.SetRoleProfile(s.defaultRoleProfile())
	s.macroMem.SetCustomRolePrompt(persona)
	fmt.Fprintln(s.out, "Persona kustom aktif.")
}

func (s *Session) handleSkillCommand(arg string) {
	name := strings.TrimSpace(arg)
	if name == "" {
		names, err := skill.ListSkills(s.cfg.SkillsDir)
		if err != nil {
			fmt.Fprintf(s.out, "Gagal membaca skills: %v\n", err)
			return
		}
		fmt.Fprintf(s.out, "Skill tersedia: %s\n", strings.Join(names, ", "))
		return
	}
	if name == "clear" {
		s.activeSkillName = "-"
		s.macroMem.SetSkillContent("")
		fmt.Fprintln(s.out, "Skill aktif dihapus.")
		return
	}

	content, err := skill.LoadSkill(s.cfg.SkillsDir, name, s.cfg.Memory.MaxSkillChars)
	if err != nil {
		fmt.Fprintf(s.out, "Gagal memuat skill: %v\n", err)
		return
	}
	s.activeSkillName = name
	s.macroMem.SetSkillContent(content)
	fmt.Fprintf(s.out, "Skill aktif: %s\n", name)
}

func (s *Session) handleNewCommand() {
	// Keep the active persona and skill across a history reset.
	customPersona := s.macroMem.CustomRolePrompt()
	s.macroMem = s.newMacroMemory()
	s.macroMem.SetCustomRolePrompt(customPersona)
	if s.activeSkillName != "" && s.activeSkillName != "-" {
		if content, err := skill.LoadSkill(s.cfg.SkillsDir, s.activeSkillName, s.cfg.Memory.MaxSkillChars); err == nil {
			s.macroMem.SetSkillContent(content)
		}
	}
	if s.cfg.SessionStatePath != "" {
		_ = os.Remove(s.cfg.SessionStatePath)
	}
	fmt.Fprintln(s.out, "Sesi baru dibuat; riwayat Macro-Memory dikosongkan.")
}

func (s *Session) handleBtwCommand(question string) {
	question = strings.TrimSpace(question)
	if question == "" {
		fmt.Fprintln(s.out, "Gunakan /btw <pertanyaan>.")
		return
	}
	if s.eng == nil {
		fmt.Fprintln(s.out, "Engine belum siap.")
		return
	}

	// /btw is a one-off query: it neither reads nor mutates Macro-Memory history.
	messages := []types.Message{
		{Role: "system", Content: s.BuildSystemPrompt()},
		{Role: "user", Content: question},
	}
	resp, err := s.eng.Generate(context.Background(), messages, []string{types.ActionFinalAnswer})
	if err != nil {
		fmt.Fprintf(s.out, "Gagal menjawab /btw: %v\n", err)
		return
	}
	fmt.Fprintln(s.out, resp.Input)
}

// Resume restores engine, modes, role, skill, and Macro-Memory from .session_state.json.
func (s *Session) Resume() error {
	if s.cfg.SessionStatePath == "" {
		return fmt.Errorf("path .session_state.json tidak dikonfigurasi")
	}
	data, err := os.ReadFile(s.cfg.SessionStatePath)
	if err != nil {
		return err
	}
	var state types.SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		// Keep the broken file for inspection instead of losing it silently.
		brokenPath := s.cfg.SessionStatePath + ".broken"
		_ = os.Rename(s.cfg.SessionStatePath, brokenPath)
		return fmt.Errorf("state sesi rusak (dipindah ke %s): %w", brokenPath, err)
	}

	if state.ActiveEngine == types.EngineLocal || state.ActiveEngine == types.EngineCloud {
		if err := s.switchEngine(state.ActiveEngine); err != nil {
			return err
		}
	}
	if state.SessionMode == types.SessionTalk || state.SessionMode == types.SessionPlan || state.SessionMode == types.SessionWork {
		s.sessionMode = state.SessionMode
	}
	if state.SecurityMode == types.ModeAuto || state.SecurityMode == types.ModeCopilot {
		s.securityMode = state.SecurityMode
		s.toolExec.SecurityMode = state.SecurityMode
	}

	s.activeSkillName = "-"
	s.activeRoleName = types.DefaultRoleName
	if state.ActiveRole != "" && state.ActiveRole != types.CustomRoleName {
		if _, ok := s.roles[state.ActiveRole]; ok {
			s.activeRoleName = state.ActiveRole
		}
	}
	s.macroMem = s.newMacroMemory()
	if state.CustomPrompt != "" {
		s.activeRoleName = types.CustomRoleName
		s.macroMem.SetCustomRolePrompt(state.CustomPrompt)
	}

	if state.ActiveSkill != "" && state.ActiveSkill != "-" {
		if content, err := skill.LoadSkill(s.cfg.SkillsDir, state.ActiveSkill, s.cfg.Memory.MaxSkillChars); err == nil {
			s.activeSkillName = state.ActiveSkill
			s.macroMem.SetSkillContent(content)
		}
	}
	s.macroMem.SetApprovedPlan(state.ApprovedPlan)
	for _, pair := range state.Pairs {
		s.macroMem.AddPair(pair)
	}
	return nil
}

// RunTurn runs one full user turn: a single answer outside /work, or the
// supervisor-worker loop inside /work.
func (s *Session) RunTurn(ctx context.Context, input string) (string, error) {
	if s.eng == nil {
		return "", fmt.Errorf("engine belum siap")
	}
	return s.runner().RunTurn(ctx, input)
}

// RunWorker executes one sterile worker task.
func (s *Session) RunWorker(ctx context.Context, taskInstruction string) (string, error) {
	return s.runner().RunWorker(ctx, taskInstruction)
}

// runner snapshots the current session state for the agent loop.
func (s *Session) runner() *agent.Runner {
	return &agent.Runner{
		Engine:      s.eng,
		Memory:      s.macroMem,
		Tools:       s.toolExec,
		Config:      s.cfg,
		SessionMode: s.sessionMode,
		OnProgress:  s.persist,
	}
}

// persist writes a checkpoint and reports failures instead of losing them.
func (s *Session) persist() {
	if err := s.saveState(); err != nil {
		fmt.Fprintf(s.out, "Peringatan: gagal menyimpan sesi: %v\n", err)
	}
}

func (s *Session) newMacroMemory() *memory.OrchestratorMemory {
	profile := s.currentRoleProfile()
	maxPairs := s.cfg.Memory.OrchMaxHistoryPairs
	if maxPairs <= 0 {
		maxPairs = types.DefaultOrchMaxHistoryPairs
	}
	threshold := s.cfg.Memory.MacroCompressionThreshold
	if threshold <= 0 {
		threshold = types.DefaultMacroCompressionThresh
	}

	mem := memory.NewOrchestratorMemory(maxPairs, threshold, s.cfg.ModePrompts, profile)
	return mem
}

func (s *Session) currentRoleProfile() types.RoleProfile {
	if profile, ok := s.roles[s.activeRoleName]; ok {
		return profile
	}
	return s.defaultRoleProfile()
}

func (s *Session) defaultRoleProfile() types.RoleProfile {
	if profile, ok := s.roles[types.DefaultRoleName]; ok {
		return profile
	}
	return config.DefaultRoleProfile()
}

func (s *Session) saveState() error {
	if s.statelessMode || s.cfg == nil || s.cfg.SessionStatePath == "" {
		return nil
	}

	state := types.SessionState{
		ActiveEngine: s.activeEngineType,
		SecurityMode: s.securityMode,
		SessionMode:  s.sessionMode,
		ActiveRole:   s.activeRoleName,
		CustomPrompt: s.macroMem.CustomRolePrompt(),
		ActiveSkill:  s.activeSkillName,
		ApprovedPlan: s.macroMem.ApprovedPlan(),
		Pairs:        s.macroMem.Pairs(),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	// Write to a temp file and rename so a crash cannot leave a half-written state.
	dir := filepath.Dir(s.cfg.SessionStatePath)
	tmp, err := os.CreateTemp(dir, ".session_state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, s.cfg.SessionStatePath); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func timeoutDuration(cfg *config.Config) time.Duration {
	timeout := time.Duration(cfg.Engine.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(types.DefaultTimeoutSeconds) * time.Second
	}
	return timeout
}
