package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"omyang/internal/bootstrap"
	"omyang/internal/types"
)

type Config struct {
	Engine         EngineSettings   `json:"engine_settings"`
	Memory         MemoryManagement `json:"memory_management"`
	CircuitBreaker CircuitBreakers  `json:"circuit_breakers"`
	Security       SecuritySettings `json:"security_settings"`
	ModePrompts    ModePrompts      `json:"mode_prompts"`
	WorkerPrompt   WorkerPromptCfg  `json:"worker_prompt"`
	DefaultSession string           `json:"default_session_mode"` // "talk", "plan", atau "work"
	DefaultRole    string           `json:"default_role"`         // "default"

	// Runtime-only paths (tidak diserialisasi ke JSON)
	BaseDir          string `json:"-"`
	ConfigPath       string `json:"-"`
	RolesPath        string `json:"-"`
	SessionStatePath string `json:"-"`
	ModelsDir        string `json:"-"`
	SkillsDir        string `json:"-"`
	ScriptsDir       string `json:"-"`
	WorkspaceDir     string `json:"-"`
}

type EngineSettings struct {
	ActiveEngine    string `json:"active_engine"`     // "local" atau "cloud"
	LlamaServerBin  string `json:"llama_server_bin"`  // Default: "llama-server"
	LocalModelPath  string `json:"local_model_path"`  // Default: "auto"
	LocalHost       string `json:"local_host"`        // Default: "127.0.0.1"
	LocalPort       int    `json:"local_port"`        // Default: 8080
	PromptCachePath string `json:"prompt_cache_path"` // Default: "./.omyang.prompt.cache"
	CloudAPIKey     string `json:"cloud_api_key"`     // Default: "YOUR_GEMINI_API_KEY_HERE"
	CloudModel      string `json:"cloud_model"`       // Default: "gemini-2.5-flash"
	TimeoutSeconds  int    `json:"timeout_seconds"`   // Default: 30
}

type MemoryManagement struct {
	OrchMaxHistoryPairs       int `json:"orchestrator_max_history_pairs"`
	MacroCompressionThreshold int `json:"macro_memory_compression_threshold"`
	TruncationHeadChars       int `json:"truncation_head_chars"`
	TruncationTailChars       int `json:"truncation_tail_chars"`
	MaxSkillChars             int `json:"max_skill_chars"`
}

type CircuitBreakers struct {
	OrchestratorMaxSteps    int `json:"orchestrator_max_steps"`
	WorkerMaxSteps          int `json:"worker_max_steps"`
	MinThoughtLength        int `json:"min_thought_length"`
	MaxDuplicateDelegations int `json:"max_duplicate_delegations"`
}

type SecuritySettings struct {
	DefaultMode          string   `json:"default_mode"`           // "copilot" atau "auto"
	AllowedDirectories   []string `json:"allowed_directories"`    // Default: ["./workspace"]
	BlockedCommandsRegex []string `json:"blocked_commands_regex"` // Pola regex perintah bash terlarang
}

type ModePrompts struct {
	Talk string `json:"talk"`
	Plan string `json:"plan"`
	Work string `json:"work"`
}

type WorkerPromptCfg struct {
	SystemPrompt string          `json:"system_prompt"`
	FewShot      []types.Message `json:"few_shot"`
}

type CLIOptions struct {
	EngineOverride string
	ModeOverride   string
	ModelOverride  string
	RoleOverride   string
	SkillOverride  string
	InitialPrompt  string
}

// DefaultRoleProfile returns the internal fallback role profile for Omyang.
func DefaultRoleProfile() types.RoleProfile {
	return types.RoleProfile{
		SystemPrompt: "Anda adalah Omyang, asisten AI eksekutor terminal yang ringkas, aman, dan efisien.",
		FewShot:      []types.Message{},
	}
}

// LoadConfig ensures ecosystem files, reads config.json, applies CLI overrides, and sets runtime paths.
func LoadConfig(baseDir string, opts CLIOptions) (*Config, []string, error) {
	warnings, err := bootstrap.EnsureEcosystem(baseDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to bootstrap ecosystem: %w", err)
	}

	configPath := filepath.Join(baseDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, warnings, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, warnings, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	// Runtime-only absolute/resolved paths
	cfg.BaseDir = baseDir
	cfg.ConfigPath = configPath
	cfg.RolesPath = filepath.Join(baseDir, "roles.json")
	cfg.SessionStatePath = filepath.Join(baseDir, ".session_state.json")
	cfg.ModelsDir = filepath.Join(baseDir, "models")
	cfg.SkillsDir = filepath.Join(baseDir, "skills")
	cfg.ScriptsDir = filepath.Join(baseDir, "scripts")
	cfg.WorkspaceDir = filepath.Join(baseDir, "workspace")

	// Apply CLI overrides
	if opts.EngineOverride != "" {
		cfg.Engine.ActiveEngine = opts.EngineOverride
	}
	if opts.ModelOverride != "" {
		cfg.Engine.LocalModelPath = opts.ModelOverride
	}
	if opts.RoleOverride != "" {
		cfg.DefaultRole = opts.RoleOverride
	}
	if opts.ModeOverride != "" {
		if opts.ModeOverride == types.SessionTalk || opts.ModeOverride == types.SessionPlan || opts.ModeOverride == types.SessionWork {
			cfg.DefaultSession = opts.ModeOverride
		} else {
			cfg.Security.DefaultMode = opts.ModeOverride
		}
	}

	if err := Validate(&cfg); err != nil {
		return nil, warnings, err
	}

	return &cfg, warnings, nil
}

// Validate rejects unknown enum values and fills empty settings with defaults.
// Callers can trust the returned config instead of re-applying defaults.
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	switch cfg.Engine.ActiveEngine {
	case "":
		cfg.Engine.ActiveEngine = types.EngineLocal
	case types.EngineLocal, types.EngineCloud:
	default:
		return fmt.Errorf("engine_settings.active_engine tidak dikenal: %q (gunakan local atau cloud)", cfg.Engine.ActiveEngine)
	}

	switch cfg.Security.DefaultMode {
	case "":
		cfg.Security.DefaultMode = types.ModeCopilot
	case types.ModeAuto, types.ModeCopilot:
	default:
		return fmt.Errorf("security_settings.default_mode tidak dikenal: %q (gunakan auto atau copilot)", cfg.Security.DefaultMode)
	}

	switch cfg.DefaultSession {
	case "":
		cfg.DefaultSession = types.SessionTalk
	case types.SessionTalk, types.SessionPlan, types.SessionWork:
	default:
		return fmt.Errorf("default_session_mode tidak dikenal: %q (gunakan talk, plan, atau work)", cfg.DefaultSession)
	}

	if cfg.DefaultRole == "" {
		cfg.DefaultRole = types.DefaultRoleName
	}
	if len(cfg.Security.AllowedDirectories) == 0 {
		cfg.Security.AllowedDirectories = []string{"./workspace"}
	}
	return nil
}

// LoadRoles reads roles.json. On error, it returns a map containing at least DefaultRoleProfile().
func LoadRoles(rolesPath string) (map[string]types.RoleProfile, error) {
	fallbackMap := map[string]types.RoleProfile{
		types.DefaultRoleName: DefaultRoleProfile(),
	}

	data, err := os.ReadFile(rolesPath)
	if err != nil {
		return fallbackMap, fmt.Errorf("failed to read roles file %s: %w", rolesPath, err)
	}

	roles := make(map[string]types.RoleProfile)
	if err := json.Unmarshal(data, &roles); err != nil {
		return fallbackMap, fmt.Errorf("failed to parse roles file %s: %w", rolesPath, err)
	}

	if _, ok := roles[types.DefaultRoleName]; !ok {
		roles[types.DefaultRoleName] = DefaultRoleProfile()
	}

	return roles, nil
}

// DiscoverLocalModel discovers a .gguf model in modelsDir or validates configuredPath.
func DiscoverLocalModel(modelsDir string, configuredPath string) (string, error) {
	if configuredPath != "" && configuredPath != types.AutoModelPath {
		if _, err := os.Stat(configuredPath); err == nil {
			return configuredPath, nil
		}
		relPath := filepath.Join(modelsDir, configuredPath)
		if _, err := os.Stat(relPath); err == nil {
			return relPath, nil
		}
		return "", fmt.Errorf("model file not found: %s", configuredPath)
	}

	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		return "", fmt.Errorf("failed to read models directory %s: %w", modelsDir, err)
	}

	var ggufFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".gguf") {
			ggufFiles = append(ggufFiles, entry.Name())
		}
	}

	sort.Strings(ggufFiles)

	if len(ggufFiles) == 0 {
		return "", fmt.Errorf("folder ./models/ kosong. Silakan letakkan minimal 1 file .gguf di dalam folder ./models/")
	}

	if len(ggufFiles) == 1 {
		return filepath.Join(modelsDir, ggufFiles[0]), nil
	}

	return "", fmt.Errorf("ditemukan lebih dari 1 file model di folder %s: %s. Silakan tentukan model dengan flag --model ./models/<nama>.gguf atau edit config.json", modelsDir, strings.Join(ggufFiles, ", "))
}

// ValidateCloudKey returns an error if CloudAPIKey is empty or remains set to the dummy default.
func ValidateCloudKey(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if cfg.Engine.CloudAPIKey == "" || cfg.Engine.CloudAPIKey == types.DummyGeminiAPIKey {
		return fmt.Errorf("kunci API cloud belum dikonfigurasi. Silakan atur cloud_api_key di config.json")
	}
	return nil
}
