package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"omyang/internal/types"
)

func writeSkillFile(t *testing.T, dir string, name string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write skill file %s: %v", name, err)
	}
}

func TestListSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "beta.md", "beta content")
	writeSkillFile(t, dir, "alpha.md", "alpha content")
	writeSkillFile(t, dir, "notes.txt", "not a skill")
	if err := os.Mkdir(filepath.Join(dir, "nested.md"), 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	got, err := ListSkills(dir)
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}
	want := []string{"alpha", "beta"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected %v, got %v", want, got)
		}
	}

	if _, err := ListSkills(filepath.Join(dir, "missing")); err == nil {
		t.Error("expected error for missing skills directory, got nil")
	}
}

func TestLoadSkill(t *testing.T) {
	dir := t.TempDir()
	content := "# Termux API\n\nGunakan `termux-battery-status`."
	writeSkillFile(t, dir, "termux_api.md", content)

	got, err := LoadSkill(dir, "termux_api", types.DefaultMaxSkillChars)
	if err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	if got != content {
		t.Errorf("expected %q, got %q", content, got)
	}

	if _, err := LoadSkill(dir, "does_not_exist", types.DefaultMaxSkillChars); err == nil {
		t.Error("expected error for missing skill, got nil")
	}

	traversalNames := []string{"../../etc/passwd", "nested/evil", "..", ""}
	for _, name := range traversalNames {
		if _, err := LoadSkill(dir, name, types.DefaultMaxSkillChars); err == nil {
			t.Errorf("expected error for unsafe skill name %q, got nil", name)
		}
	}
}

func TestLoadSkillTruncatesMaxChars(t *testing.T) {
	dir := t.TempDir()

	t.Run("ascii truncation keeps prefix and limit", func(t *testing.T) {
		content := strings.Repeat("a", 100)
		writeSkillFile(t, dir, "long.md", content)

		got, err := LoadSkill(dir, "long", 10)
		if err != nil {
			t.Fatalf("LoadSkill failed: %v", err)
		}
		if len(got) != 10 {
			t.Errorf("expected length 10, got %d", len(got))
		}
		if !strings.HasPrefix(content, got) {
			t.Errorf("expected prefix of original content, got %q", got)
		}
	})

	t.Run("multibyte truncation stays valid UTF-8", func(t *testing.T) {
		content := strings.Repeat("é", 100)
		writeSkillFile(t, dir, "unicode.md", content)

		got, err := LoadSkill(dir, "unicode", 7)
		if err != nil {
			t.Fatalf("LoadSkill failed: %v", err)
		}
		if !utf8.ValidString(got) {
			t.Errorf("expected valid UTF-8, got %q", got)
		}
		if utf8.RuneCountInString(got) != 7 {
			t.Errorf("expected 7 runes, got %d", utf8.RuneCountInString(got))
		}
	})

	t.Run("content within limit is unchanged", func(t *testing.T) {
		content := strings.Repeat("b", 10)
		writeSkillFile(t, dir, "exact.md", content)

		got, err := LoadSkill(dir, "exact", 10)
		if err != nil {
			t.Fatalf("LoadSkill failed: %v", err)
		}
		if got != content {
			t.Errorf("expected %q, got %q", content, got)
		}
	})

	t.Run("zero maxChars falls back to default limit", func(t *testing.T) {
		content := strings.Repeat("c", 20)
		writeSkillFile(t, dir, "zero.md", content)

		got, err := LoadSkill(dir, "zero", 0)
		if err != nil {
			t.Fatalf("LoadSkill failed: %v", err)
		}
		if got != content {
			t.Errorf("expected %q, got %q", content, got)
		}
	})
}
