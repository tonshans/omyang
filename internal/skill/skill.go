package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"omyang/internal/types"
)

// ListSkills returns sorted .md skill names (without extension) found in skillsDir.
func ListSkills(skillsDir string) ([]string, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read skills directory %s: %w", skillsDir, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.EqualFold(filepath.Ext(name), ".md") {
			continue
		}
		names = append(names, name[:len(name)-len(filepath.Ext(name))])
	}

	sort.Strings(names)
	return names, nil
}

// LoadSkill reads <skillsDir>/<name>.md and truncates it to maxChars characters.
func LoadSkill(skillsDir string, name string, maxChars int) (string, error) {
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		return "", fmt.Errorf("invalid skill name %q", name)
	}

	data, err := os.ReadFile(filepath.Join(skillsDir, name+".md"))
	if err != nil {
		return "", fmt.Errorf("failed to read skill %q: %w", name, err)
	}

	if maxChars <= 0 {
		maxChars = types.DefaultMaxSkillChars
	}

	runes := []rune(string(data))
	if len(runes) > maxChars {
		runes = runes[:maxChars]
	}
	return string(runes), nil
}
