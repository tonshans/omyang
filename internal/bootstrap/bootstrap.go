package bootstrap

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed assets/*
var assetsFS embed.FS

// EnsureEcosystem ensures required directories and default files exist in baseDir.
// If config.json or roles.json are missing, default files from assets are created.
// If config.json or roles.json are corrupted, they are renamed to .broken and recreated.
func EnsureEcosystem(baseDir string) ([]string, error) {
	var warnings []string

	// 1. Create required directories
	requiredDirs := []string{"models", "skills", "scripts", "workspace"}
	for _, dir := range requiredDirs {
		dirPath := filepath.Join(baseDir, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dirPath, err)
		}
	}

	// 2. Extract starter kit to skills/ if empty
	skillsDir := filepath.Join(baseDir, "skills")
	if err := extractIfEmpty(skillsDir, "assets/skills", 0644); err != nil {
		return nil, fmt.Errorf("failed to extract starter skills: %w", err)
	}

	// 3. Extract starter kit to scripts/ if empty (with 0755 for .sh)
	scriptsDir := filepath.Join(baseDir, "scripts")
	if err := extractIfEmpty(scriptsDir, "assets/scripts", 0755); err != nil {
		return nil, fmt.Errorf("failed to extract starter scripts: %w", err)
	}

	// 4. Check and ensure config.json
	configWarnings, err := ensureJSONFile(baseDir, "config.json", "assets/default_config.json")
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, configWarnings...)

	// 5. Check and ensure roles.json
	rolesWarnings, err := ensureJSONFile(baseDir, "roles.json", "assets/default_roles.json")
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, rolesWarnings...)

	return warnings, nil
}

func extractIfEmpty(targetDir string, assetSubDir string, defaultPerm os.FileMode) error {
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return nil
	}

	assetEntries, err := fs.ReadDir(assetsFS, assetSubDir)
	if err != nil {
		return err
	}

	for _, entry := range assetEntries {
		if entry.IsDir() {
			continue
		}

		assetPath := assetSubDir + "/" + entry.Name()
		data, err := assetsFS.ReadFile(assetPath)
		if err != nil {
			return err
		}

		perm := defaultPerm
		if strings.HasSuffix(entry.Name(), ".sh") {
			perm = 0755
		}

		dstPath := filepath.Join(targetDir, entry.Name())
		if err := os.WriteFile(dstPath, data, perm); err != nil {
			return err
		}
	}
	return nil
}

func ensureJSONFile(baseDir string, filename string, assetPath string) ([]string, error) {
	filePath := filepath.Join(baseDir, filename)
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		defaultData, err := assetsFS.ReadFile(assetPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read embedded asset %s: %w", assetPath, err)
		}
		if err := os.WriteFile(filePath, defaultData, 0644); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", filePath, err)
		}
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		brokenPath := filePath + ".broken"
		if err := os.Rename(filePath, brokenPath); err != nil {
			return nil, fmt.Errorf("failed to rename broken %s to %s: %w", filePath, brokenPath, err)
		}

		defaultData, err := assetsFS.ReadFile(assetPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read embedded asset %s: %w", assetPath, err)
		}
		if err := os.WriteFile(filePath, defaultData, 0644); err != nil {
			return nil, fmt.Errorf("failed to restore default %s: %w", filePath, err)
		}

		warning := fmt.Sprintf("%s was corrupted and renamed to %s.broken; default recreated: %v", filename, filename, err)
		return []string{warning}, nil
	}

	return nil, nil
}
