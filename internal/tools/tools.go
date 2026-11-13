package tools

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"omyang/internal/types"
)

// Executor runs Worker tools (read, write, bash) inside the workspace sandbox.
// shortcut: bash is NOT confined to the workspace (cwd only) and the denylist is
// best-effort; use copilot mode or an OS-level sandbox when untrusted models run.
type Executor struct {
	WorkspaceDir    string
	AllowedDirs     []string
	BlockedPatterns []string
	SecurityMode    string
	Timeout         time.Duration
	Reader          *bufio.Reader
	Out             io.Writer
}

// ValidateWorkspacePath resolves targetPath and rejects anything outside allowedDirs.
// Symlinks are resolved before the containment check so a link inside the
// workspace cannot point at files outside it.
func ValidateWorkspacePath(targetPath string, workspaceDir string, allowedDirs []string) (string, error) {
	wsAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("akses ditolak: gagal meresolusi workspace: %w", err)
	}
	wsAbs = filepath.Clean(wsAbs)
	baseDir := filepath.Dir(wsAbs)

	var resolved string
	if filepath.IsAbs(targetPath) {
		resolved = filepath.Clean(targetPath)
	} else if targetPath == "workspace" || strings.HasPrefix(targetPath, "workspace"+string(filepath.Separator)) {
		resolved = filepath.Clean(filepath.Join(baseDir, targetPath))
	} else {
		resolved = filepath.Clean(filepath.Join(wsAbs, targetPath))
	}
	resolvedReal := evalExisting(resolved)

	roots := allowedDirs
	if len(roots) == 0 {
		roots = []string{wsAbs}
	}
	for _, root := range roots {
		absRoot := root
		if !filepath.IsAbs(absRoot) {
			absRoot = filepath.Join(baseDir, absRoot)
		}
		if isWithin(resolvedReal, evalExisting(filepath.Clean(absRoot))) {
			return resolved, nil
		}
	}

	return "", fmt.Errorf("akses ditolak: path %q berada di luar direktori yang diizinkan", targetPath)
}

// evalExisting resolves symlinks on the deepest existing ancestor of path, so a
// non-existent write target cannot escape through a symlinked parent directory.
func evalExisting(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	return filepath.Join(evalExisting(parent), filepath.Base(path))
}

// isWithin reports whether path is root itself or a descendant of root.
func isWithin(path string, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// IsCommandBlocked checks cmdStr against blockedPatterns and returns the matching pattern.
func IsCommandBlocked(cmdStr string, blockedPatterns []string) (bool, string) {
	for _, pattern := range blockedPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(cmdStr) {
			return true, fmt.Sprintf("perintah diblokir oleh pola keamanan: %s", pattern)
		}
	}
	return false, ""
}

// Execute runs read, write, or bash. Write input is "<path>\n<content>".
func (e *Executor) Execute(action string, input string) (string, error) {
	switch action {
	case types.ActionRead:
		path, err := ValidateWorkspacePath(input, e.WorkspaceDir, e.AllowedDirs)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("gagal membaca file %s: %w", input, err)
		}
		return string(data), nil

	case types.ActionWrite:
		parts := strings.SplitN(input, "\n", 2)
		path, err := ValidateWorkspacePath(parts[0], e.WorkspaceDir, e.AllowedDirs)
		if err != nil {
			return "", err
		}
		content := ""
		if len(parts) > 1 {
			content = parts[1]
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("gagal menulis file %s: %w", parts[0], err)
		}
		return fmt.Sprintf("berhasil menulis %d byte ke %s", len(content), parts[0]), nil

	case types.ActionBash:
		return e.executeBash(input)

	default:
		return "", fmt.Errorf("aksi alat tidak dikenal: %s", action)
	}
}

func (e *Executor) executeBash(command string) (string, error) {
	if blocked, reason := IsCommandBlocked(command, e.BlockedPatterns); blocked {
		return "", fmt.Errorf("%s", reason)
	}

	if e.SecurityMode == types.ModeCopilot {
		if e.Reader == nil {
			return "", fmt.Errorf("konfirmasi copilot tidak tersedia (tanpa input interaktif); jalankan interaktif atau gunakan /mode auto")
		}
		out := e.Out
		if out == nil {
			out = os.Stdout
		}
		fmt.Fprintf(out, "Jalankan perintah %q? [y/n] ", command)
		line, err := e.Reader.ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("gagal membaca konfirmasi: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "ya", "yes":
			// confirmed
		default:
			return "Perintah dibatalkan oleh pengguna.", nil
		}
	}

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = time.Duration(types.DefaultTimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = e.WorkspaceDir
	// Kill leaves no orphan child holding the pipe open past the timeout.
	cmd.WaitDelay = 500 * time.Millisecond

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(output), fmt.Errorf("perintah melewati batas waktu %s: %w", timeout, ctx.Err())
	}
	if err != nil {
		return string(output), fmt.Errorf("perintah gagal: %w", err)
	}
	return string(output), nil
}
