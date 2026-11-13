package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omyang/internal/types"
)

func TestLoadConfig_BasicAndCLIOptions(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Basic load without options
	cfg, warnings, err := LoadConfig(tempDir, CLIOptions{})
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	_ = warnings

	// Check runtime paths
	if cfg.BaseDir != tempDir {
		t.Errorf("expected BaseDir %s, got %s", tempDir, cfg.BaseDir)
	}
	if cfg.ConfigPath != filepath.Join(tempDir, "config.json") {
		t.Errorf("expected ConfigPath %s, got %s", filepath.Join(tempDir, "config.json"), cfg.ConfigPath)
	}
	if cfg.RolesPath != filepath.Join(tempDir, "roles.json") {
		t.Errorf("expected RolesPath %s, got %s", filepath.Join(tempDir, "roles.json"), cfg.RolesPath)
	}
	if cfg.SessionStatePath != filepath.Join(tempDir, ".session_state.json") {
		t.Errorf("expected SessionStatePath %s, got %s", filepath.Join(tempDir, ".session_state.json"), cfg.SessionStatePath)
	}
	if cfg.ModelsDir != filepath.Join(tempDir, "models") {
		t.Errorf("expected ModelsDir %s, got %s", filepath.Join(tempDir, "models"), cfg.ModelsDir)
	}
	if cfg.SkillsDir != filepath.Join(tempDir, "skills") {
		t.Errorf("expected SkillsDir %s, got %s", filepath.Join(tempDir, "skills"), cfg.SkillsDir)
	}
	if cfg.ScriptsDir != filepath.Join(tempDir, "scripts") {
		t.Errorf("expected ScriptsDir %s, got %s", filepath.Join(tempDir, "scripts"), cfg.ScriptsDir)
	}
	if cfg.WorkspaceDir != filepath.Join(tempDir, "workspace") {
		t.Errorf("expected WorkspaceDir %s, got %s", filepath.Join(tempDir, "workspace"), cfg.WorkspaceDir)
	}

	// 2. Load with CLIOptions override
	opts := CLIOptions{
		EngineOverride: types.EngineCloud,
		ModeOverride:   types.ModeAuto,
		ModelOverride:  "./models/custom.gguf",
		RoleOverride:   "sysadmin",
	}

	cfgOverride, _, err := LoadConfig(tempDir, opts)
	if err != nil {
		t.Fatalf("LoadConfig with opts failed: %v", err)
	}

	if cfgOverride.Engine.ActiveEngine != types.EngineCloud {
		t.Errorf("expected ActiveEngine %s, got %s", types.EngineCloud, cfgOverride.Engine.ActiveEngine)
	}
	if cfgOverride.Security.DefaultMode != types.ModeAuto {
		t.Errorf("expected DefaultMode %s, got %s", types.ModeAuto, cfgOverride.Security.DefaultMode)
	}
	if cfgOverride.Engine.LocalModelPath != "./models/custom.gguf" {
		t.Errorf("expected LocalModelPath ./models/custom.gguf, got %s", cfgOverride.Engine.LocalModelPath)
	}
	if cfgOverride.DefaultRole != "sysadmin" {
		t.Errorf("expected DefaultRole sysadmin, got %s", cfgOverride.DefaultRole)
	}

	// Session mode override test
	optsSession := CLIOptions{
		ModeOverride: types.SessionWork,
	}
	cfgSession, _, err := LoadConfig(tempDir, optsSession)
	if err != nil {
		t.Fatalf("LoadConfig with session override failed: %v", err)
	}
	if cfgSession.DefaultSession != types.SessionWork {
		t.Errorf("expected DefaultSession %s, got %s", types.SessionWork, cfgSession.DefaultSession)
	}
}

func TestValidateFillsDefaults(t *testing.T) {
	cfg := &Config{}
	if err := Validate(cfg); err != nil {
		t.Fatalf("config kosong harus valid: %v", err)
	}
	if cfg.Engine.ActiveEngine != types.EngineLocal {
		t.Errorf("ActiveEngine = %q, want local", cfg.Engine.ActiveEngine)
	}
	if cfg.Security.DefaultMode != types.ModeCopilot {
		t.Errorf("DefaultMode = %q, want copilot", cfg.Security.DefaultMode)
	}
	if cfg.DefaultSession != types.SessionTalk {
		t.Errorf("DefaultSession = %q, want talk", cfg.DefaultSession)
	}
	if cfg.DefaultRole != types.DefaultRoleName {
		t.Errorf("DefaultRole = %q, want default", cfg.DefaultRole)
	}
	if len(cfg.Security.AllowedDirectories) != 1 || cfg.Security.AllowedDirectories[0] != "./workspace" {
		t.Errorf("AllowedDirectories = %v, want [./workspace]", cfg.Security.AllowedDirectories)
	}
}

func TestValidateRejectsUnknownEnums(t *testing.T) {
	cases := map[string]*Config{
		"engine":  {Engine: EngineSettings{ActiveEngine: "bogus"}},
		"mode":    {Security: SecuritySettings{DefaultMode: "bogus"}},
		"session": {DefaultSession: "bogus"},
	}
	for name, cfg := range cases {
		if err := Validate(cfg); err == nil {
			t.Errorf("Validate harus menolak %s tidak dikenal", name)
		}
	}
}

func TestLoadConfig_RejectsUnknownCLIEngine(t *testing.T) {
	if _, _, err := LoadConfig(t.TempDir(), CLIOptions{EngineOverride: "bogus"}); err == nil {
		t.Error("LoadConfig harus menolak --engine yang tidak dikenal")
	}
}

func TestLoadRoles_Fallback(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Valid roles.json
	rolesPath := filepath.Join(tempDir, "roles.json")
	validJSON := `{
		"default": {
			"system_prompt": "prompt default",
			"few_shot": []
		},
		"sysadmin": {
			"system_prompt": "prompt sysadmin",
			"few_shot": []
		}
	}`
	if err := os.WriteFile(rolesPath, []byte(validJSON), 0644); err != nil {
		t.Fatalf("failed to write roles.json: %v", err)
	}

	roles, err := LoadRoles(rolesPath)
	if err != nil {
		t.Fatalf("expected no error for valid roles.json, got: %v", err)
	}
	if len(roles) < 2 {
		t.Errorf("expected at least 2 roles, got %d", len(roles))
	}
	if roles["default"].SystemPrompt != "prompt default" {
		t.Errorf("unexpected default role prompt: %s", roles["default"].SystemPrompt)
	}

	// 2. Corrupted roles.json should return error and fallback DefaultRoleProfile
	corruptedPath := filepath.Join(tempDir, "corrupted_roles.json")
	if err := os.WriteFile(corruptedPath, []byte("{broken json"), 0644); err != nil {
		t.Fatalf("failed to write corrupted roles.json: %v", err)
	}

	fallbackRoles, err := LoadRoles(corruptedPath)
	if err == nil {
		t.Fatal("expected error for corrupted roles.json, got nil")
	}
	defaultProfile, ok := fallbackRoles[types.DefaultRoleName]
	if !ok {
		t.Fatalf("expected fallback roles map to contain %q key", types.DefaultRoleName)
	}
	expectedDefault := DefaultRoleProfile()
	if defaultProfile.SystemPrompt != expectedDefault.SystemPrompt {
		t.Errorf("expected fallback system prompt %q, got %q", expectedDefault.SystemPrompt, defaultProfile.SystemPrompt)
	}

	// 3. Non-existent roles.json
	missingRoles, err := LoadRoles(filepath.Join(tempDir, "missing.json"))
	if err == nil {
		t.Fatal("expected error for non-existent roles.json, got nil")
	}
	if _, ok := missingRoles[types.DefaultRoleName]; !ok {
		t.Fatalf("expected missing roles map to contain %q key", types.DefaultRoleName)
	}
}

func TestValidateCloudKey(t *testing.T) {
	// 1. Nil config
	if err := ValidateCloudKey(nil); err == nil {
		t.Error("expected error for nil config, got nil")
	}

	cfg := &Config{}

	// 2. Empty key
	cfg.Engine.CloudAPIKey = ""
	if err := ValidateCloudKey(cfg); err == nil {
		t.Error("expected error for empty CloudAPIKey, got nil")
	}

	// 3. Dummy key
	cfg.Engine.CloudAPIKey = types.DummyGeminiAPIKey
	if err := ValidateCloudKey(cfg); err == nil {
		t.Error("expected error for dummy CloudAPIKey, got nil")
	}

	// 4. Valid key
	cfg.Engine.CloudAPIKey = "AIzaSyD-valid-key-12345"
	if err := ValidateCloudKey(cfg); err != nil {
		t.Errorf("expected valid key to pass, got: %v", err)
	}
}

func TestDiscoverLocalModel(t *testing.T) {
	tempDir := t.TempDir()
	modelsDir := filepath.Join(tempDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatalf("failed to create models directory: %v", err)
	}

	// Skenario 1: 0 file .gguf
	_, err := DiscoverLocalModel(modelsDir, "auto")
	if err == nil {
		t.Fatal("expected error for 0 .gguf files, got nil")
	}
	if !strings.Contains(err.Error(), "models") || !strings.Contains(err.Error(), ".gguf") {
		t.Errorf("error message should mention models folder and .gguf, got: %v", err)
	}

	// Also test empty string configuredPath with 0 files
	_, err = DiscoverLocalModel(modelsDir, "")
	if err == nil {
		t.Fatal("expected error for empty configuredPath with 0 .gguf files, got nil")
	}

	// Skenario 2: tepat 1 file .gguf
	modelFile1 := filepath.Join(modelsDir, "llama3b.gguf")
	if err := os.WriteFile(modelFile1, []byte("dummy-model"), 0644); err != nil {
		t.Fatalf("failed to write model file: %v", err)
	}

	discovered, err := DiscoverLocalModel(modelsDir, "auto")
	if err != nil {
		t.Fatalf("expected success for 1 .gguf file, got: %v", err)
	}
	if filepath.Clean(discovered) != filepath.Clean(modelFile1) {
		t.Errorf("expected discovered path %s, got %s", modelFile1, discovered)
	}

	// Skenario 3: lebih dari 1 file .gguf
	modelFile2 := filepath.Join(modelsDir, "qwen2.5.gguf")
	if err := os.WriteFile(modelFile2, []byte("dummy-model-2"), 0644); err != nil {
		t.Fatalf("failed to write second model file: %v", err)
	}

	_, err = DiscoverLocalModel(modelsDir, "auto")
	if err == nil {
		t.Fatal("expected error for >1 .gguf files, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "llama3b.gguf") || !strings.Contains(errMsg, "qwen2.5.gguf") {
		t.Errorf("error should list both gguf files, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "--model") || !strings.Contains(errMsg, "config.json") {
		t.Errorf("error should mention --model and config.json, got: %s", errMsg)
	}

	// Explicit configuredPath (non-auto) exists
	explicitFound, err := DiscoverLocalModel(modelsDir, modelFile1)
	if err != nil {
		t.Fatalf("expected explicit path to succeed, got: %v", err)
	}
	if filepath.Clean(explicitFound) != filepath.Clean(modelFile1) {
		t.Errorf("expected %s, got %s", modelFile1, explicitFound)
	}

	// Explicit configuredPath (non-auto) does not exist
	_, err = DiscoverLocalModel(modelsDir, filepath.Join(modelsDir, "notfound.gguf"))
	if err == nil {
		t.Fatal("expected error for non-existent explicit model path, got nil")
	}
}
