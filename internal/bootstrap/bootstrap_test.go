package bootstrap_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omyang/internal/bootstrap"
)

func TestEnsureEcosystem_CleanDirectory(t *testing.T) {
	tempDir := t.TempDir()

	warnings, err := bootstrap.EnsureEcosystem(tempDir)
	if err != nil {
		t.Fatalf("unexpected error from EnsureEcosystem: %v", err)
	}

	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings in clean directory, got %d: %v", len(warnings), warnings)
	}

	// 1. Verify required directories
	requiredDirs := []string{"models", "skills", "scripts", "workspace"}
	for _, dir := range requiredDirs {
		dirPath := filepath.Join(tempDir, dir)
		info, err := os.Stat(dirPath)
		if err != nil {
			t.Errorf("expected directory %s to exist, error: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", dir)
		}
	}

	// 2. Verify config.json
	configPath := filepath.Join(tempDir, "config.json")
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected config.json to exist: %v", err)
	}
	var configMap map[string]any
	if err := json.Unmarshal(configBytes, &configMap); err != nil {
		t.Errorf("expected valid JSON in config.json: %v", err)
	}

	// 3. Verify roles.json
	rolesPath := filepath.Join(tempDir, "roles.json")
	rolesBytes, err := os.ReadFile(rolesPath)
	if err != nil {
		t.Fatalf("expected roles.json to exist: %v", err)
	}
	var rolesMap map[string]any
	if err := json.Unmarshal(rolesBytes, &rolesMap); err != nil {
		t.Errorf("expected valid JSON in roles.json: %v", err)
	}

	// 4. Verify starter kit skills extracted
	expectedSkills := []string{"termux_api.md", "script_docs.md"}
	for _, skill := range expectedSkills {
		skillPath := filepath.Join(tempDir, "skills", skill)
		content, err := os.ReadFile(skillPath)
		if err != nil {
			t.Errorf("expected skill %s to be extracted: %v", skill, err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("expected skill %s to be non-empty", skill)
		}
	}

	// 5. Verify starter kit scripts extracted with executable permissions (0755)
	expectedScripts := []string{"system_check.sh", "network_test.sh"}
	for _, script := range expectedScripts {
		scriptPath := filepath.Join(tempDir, "scripts", script)
		info, err := os.Stat(scriptPath)
		if err != nil {
			t.Errorf("expected script %s to be extracted: %v", script, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("expected script %s to be non-empty", script)
		}
		perm := info.Mode().Perm()
		if perm&0111 == 0 {
			t.Errorf("expected script %s to have executable permissions, got mode %o", script, perm)
		}
	}
}

func TestEnsureEcosystem_CorruptedConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	corruptedData := []byte("{broken")

	if err := os.WriteFile(configPath, corruptedData, 0644); err != nil {
		t.Fatalf("failed to create broken config: %v", err)
	}

	warnings, err := bootstrap.EnsureEcosystem(tempDir)
	if err != nil {
		t.Fatalf("EnsureEcosystem should not return error on corrupted config: %v", err)
	}

	// Warning must be generated
	if len(warnings) == 0 {
		t.Errorf("expected warnings when config.json is corrupted, got none")
	} else {
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "config.json") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected warning mentioning config.json, got: %v", warnings)
		}
	}

	// Old file renamed to config.json.broken
	brokenPath := filepath.Join(tempDir, "config.json.broken")
	brokenBytes, err := os.ReadFile(brokenPath)
	if err != nil {
		t.Fatalf("expected config.json.broken to exist: %v", err)
	}
	if string(brokenBytes) != string(corruptedData) {
		t.Errorf("expected config.json.broken to retain corrupted data, got %s", string(brokenBytes))
	}

	// New default config.json created and valid JSON
	newConfigBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected new config.json to exist: %v", err)
	}
	var configMap map[string]any
	if err := json.Unmarshal(newConfigBytes, &configMap); err != nil {
		t.Errorf("expected newly created config.json to be valid JSON: %v", err)
	}
}

func TestEnsureEcosystem_CorruptedRoles(t *testing.T) {
	tempDir := t.TempDir()
	rolesPath := filepath.Join(tempDir, "roles.json")
	corruptedData := []byte("{broken")

	if err := os.WriteFile(rolesPath, corruptedData, 0644); err != nil {
		t.Fatalf("failed to create broken roles: %v", err)
	}

	warnings, err := bootstrap.EnsureEcosystem(tempDir)
	if err != nil {
		t.Fatalf("EnsureEcosystem should not return error on corrupted roles: %v", err)
	}

	// Warning must be generated
	if len(warnings) == 0 {
		t.Errorf("expected warnings when roles.json is corrupted, got none")
	} else {
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "roles.json") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected warning mentioning roles.json, got: %v", warnings)
		}
	}

	// Old file renamed to roles.json.broken
	brokenPath := filepath.Join(tempDir, "roles.json.broken")
	brokenBytes, err := os.ReadFile(brokenPath)
	if err != nil {
		t.Fatalf("expected roles.json.broken to exist: %v", err)
	}
	if string(brokenBytes) != string(corruptedData) {
		t.Errorf("expected roles.json.broken to retain corrupted data, got %s", string(brokenBytes))
	}

	// New default roles.json created and valid JSON
	newRolesBytes, err := os.ReadFile(rolesPath)
	if err != nil {
		t.Fatalf("expected new roles.json to exist: %v", err)
	}
	var rolesMap map[string]any
	if err := json.Unmarshal(newRolesBytes, &rolesMap); err != nil {
		t.Errorf("expected newly created roles.json to be valid JSON: %v", err)
	}
}

func TestEnsureEcosystem_BothCorrupted(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	rolesPath := filepath.Join(tempDir, "roles.json")

	if err := os.WriteFile(configPath, []byte("invalid config json"), 0644); err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}
	if err := os.WriteFile(rolesPath, []byte("invalid roles json"), 0644); err != nil {
		t.Fatalf("failed to write invalid roles: %v", err)
	}

	warnings, err := bootstrap.EnsureEcosystem(tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(warnings) < 2 {
		t.Errorf("expected at least 2 warnings, got %d: %v", len(warnings), warnings)
	}

	if _, err := os.Stat(filepath.Join(tempDir, "config.json.broken")); err != nil {
		t.Errorf("expected config.json.broken to exist")
	}
	if _, err := os.Stat(filepath.Join(tempDir, "roles.json.broken")); err != nil {
		t.Errorf("expected roles.json.broken to exist")
	}

	// Verify both new files are valid JSON
	newConfigBytes, _ := os.ReadFile(configPath)
	var cfg map[string]any
	if err := json.Unmarshal(newConfigBytes, &cfg); err != nil {
		t.Errorf("expected recreated config.json to be valid JSON: %v", err)
	}

	newRolesBytes, _ := os.ReadFile(rolesPath)
	var roles map[string]any
	if err := json.Unmarshal(newRolesBytes, &roles); err != nil {
		t.Errorf("expected recreated roles.json to be valid JSON: %v", err)
	}
}

func TestEnsureEcosystem_ExistingValidFilesNotOverwritten(t *testing.T) {
	tempDir := t.TempDir()

	// Initial bootstrap
	_, err := bootstrap.EnsureEcosystem(tempDir)
	if err != nil {
		t.Fatalf("first EnsureEcosystem failed: %v", err)
	}

	// Customize config.json
	customConfig := `{"custom_field": "preserved_value"}`
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte(customConfig), 0644); err != nil {
		t.Fatalf("failed to write custom config: %v", err)
	}

	// Add custom skill
	customSkillPath := filepath.Join(tempDir, "skills", "my_custom_skill.md")
	if err := os.WriteFile(customSkillPath, []byte("# Custom Skill"), 0644); err != nil {
		t.Fatalf("failed to write custom skill: %v", err)
	}

	// Second bootstrap
	warnings, err := bootstrap.EnsureEcosystem(tempDir)
	if err != nil {
		t.Fatalf("second EnsureEcosystem failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings on existing valid ecosystem, got: %v", warnings)
	}

	// Verify custom config was NOT overwritten
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}
	if string(content) != customConfig {
		t.Errorf("expected custom config to be preserved, got: %s", string(content))
	}

	// Verify custom skill is still present
	if _, err := os.Stat(customSkillPath); err != nil {
		t.Errorf("expected custom skill to be preserved: %v", err)
	}
}
