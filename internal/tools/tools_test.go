package tools

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"omyang/internal/types"
)

func newTestExecutor(t *testing.T) (*Executor, string) {
	t.Helper()
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	return &Executor{
		WorkspaceDir:    workspace,
		AllowedDirs:     []string{workspace},
		BlockedPatterns: []string{`rm\s+-rf\s+/`, `mkfs`, `:\(\)\s*\{`},
		SecurityMode:    types.ModeAuto,
		Timeout:         2 * time.Second,
		Out:             io.Discard,
	}, workspace
}

func TestValidateWorkspacePathRejectsTraversal(t *testing.T) {
	ex, workspace := newTestExecutor(t)
	base := filepath.Dir(workspace)

	for _, target := range []string{"../../etc/passwd", "../config.json", "sub/../../../etc/passwd"} {
		if _, err := ValidateWorkspacePath(target, ex.WorkspaceDir, ex.AllowedDirs); err == nil {
			t.Errorf("expected traversal %q to be rejected", target)
		} else if !strings.Contains(strings.ToLower(err.Error()), "akses ditolak") {
			t.Errorf("expected explicit access-denied error for %q, got %q", target, err)
		}
	}

	// A sibling directory sharing the workspace prefix must not be treated as inside it.
	sibling := filepath.Join(base, "workspace-evil", "evil.txt")
	if err := os.MkdirAll(filepath.Dir(sibling), 0755); err != nil {
		t.Fatalf("failed to create sibling dir: %v", err)
	}
	if _, err := ValidateWorkspacePath(sibling, ex.WorkspaceDir, ex.AllowedDirs); err == nil {
		t.Errorf("expected sibling path %q to be rejected", sibling)
	}
}

func TestValidateWorkspacePathAllowsInsideWorkspace(t *testing.T) {
	ex, workspace := newTestExecutor(t)

	cases := map[string]string{
		"notes.txt":                         filepath.Join(workspace, "notes.txt"),
		"sub/dir/notes.txt":                 filepath.Join(workspace, "sub", "dir", "notes.txt"),
		"workspace/notes.txt":               filepath.Join(workspace, "notes.txt"),
		"a/../notes.txt":                    filepath.Join(workspace, "notes.txt"),
		filepath.Join(workspace, "abs.txt"): filepath.Join(workspace, "abs.txt"),
	}

	for target, want := range cases {
		got, err := ValidateWorkspacePath(target, ex.WorkspaceDir, ex.AllowedDirs)
		if err != nil {
			t.Errorf("expected %q to be allowed, got error: %v", target, err)
			continue
		}
		if got != want {
			t.Errorf("expected resolved path %q, got %q", want, got)
		}
	}

	// Empty allowedDirs falls back to workspaceDir.
	if _, err := ValidateWorkspacePath("notes.txt", ex.WorkspaceDir, nil); err != nil {
		t.Errorf("expected fallback to workspaceDir, got error: %v", err)
	}
}

func TestIsCommandBlocked(t *testing.T) {
	patterns := []string{`rm\s+-rf\s+/`, `mkfs`, `:\(\)\s*\{`}

	blocked, reason := IsCommandBlocked("rm -rf /", patterns)
	if !blocked {
		t.Fatal("expected 'rm -rf /' to be blocked")
	}
	if reason == "" {
		t.Error("expected a non-empty block reason")
	}

	if blocked, _ := IsCommandBlocked("mkfs.ext4 /dev/sda1", patterns); !blocked {
		t.Error("expected 'mkfs.ext4 /dev/sda1' to be blocked")
	}

	if blocked, _ := IsCommandBlocked("ls -la", patterns); blocked {
		t.Error("expected 'ls -la' to be allowed")
	}

	if blocked, _ := IsCommandBlocked("rm -rf ./build", patterns); blocked {
		t.Error("expected 'rm -rf ./build' to be allowed")
	}
}

func TestExecuteReadAndWriteInsideWorkspace(t *testing.T) {
	ex, workspace := newTestExecutor(t)

	out, err := ex.Execute(types.ActionWrite, "catatan.txt\nhalo dunia")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if out == "" {
		t.Error("expected a non-empty write confirmation")
	}

	onDisk, err := os.ReadFile(filepath.Join(workspace, "catatan.txt"))
	if err != nil {
		t.Fatalf("expected file written inside workspace: %v", err)
	}
	if string(onDisk) != "halo dunia" {
		t.Errorf("expected content %q, got %q", "halo dunia", string(onDisk))
	}

	out, err = ex.Execute(types.ActionRead, "catatan.txt")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if out != "halo dunia" {
		t.Errorf("expected read output %q, got %q", "halo dunia", out)
	}

	// Multi-line content is preserved after the first line (the path).
	if _, err := ex.Execute(types.ActionWrite, "multi.txt\nbaris1\nbaris2"); err != nil {
		t.Fatalf("multi-line write failed: %v", err)
	}
	out, err = ex.Execute(types.ActionRead, "multi.txt")
	if err != nil {
		t.Fatalf("multi-line read failed: %v", err)
	}
	if out != "baris1\nbaris2" {
		t.Errorf("expected multi-line content, got %q", out)
	}
}

func TestExecuteRejectsTraversalOutsideWorkspace(t *testing.T) {
	ex, workspace := newTestExecutor(t)
	base := filepath.Dir(workspace)

	if _, err := ex.Execute(types.ActionRead, "../../etc/passwd"); err == nil {
		t.Error("expected read traversal to be rejected")
	} else if !strings.Contains(strings.ToLower(err.Error()), "akses ditolak") {
		t.Errorf("expected access-denied error, got %q", err)
	}

	if _, err := ex.Execute(types.ActionWrite, "../config.json\nPWNED"); err == nil {
		t.Error("expected write traversal to be rejected")
	} else if !strings.Contains(strings.ToLower(err.Error()), "akses ditolak") {
		t.Errorf("expected access-denied error, got %q", err)
	}
	if _, err := os.Stat(filepath.Join(base, "config.json")); !os.IsNotExist(err) {
		t.Error("write traversal must not create a file outside workspace")
	}
}

func TestExecuteBash_BlockedBeforeOSExecution(t *testing.T) {
	ex, _ := newTestExecutor(t)
	ex.SecurityMode = types.ModeCopilot
	ex.Reader = bufio.NewReader(strings.NewReader("y\n"))

	marker := filepath.Join(ex.WorkspaceDir, "should-not-exist.txt")
	_, err := ex.Execute(types.ActionBash, "rm -rf / && touch should-not-exist.txt")
	if err == nil {
		t.Fatal("expected blocked command to return an error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "diblokir") {
		t.Errorf("expected explicit block message, got %q", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Error("blocked command must not execute")
	}
}

func TestExecuteBash_CopilotCancel(t *testing.T) {
	ex, _ := newTestExecutor(t)
	ex.SecurityMode = types.ModeCopilot
	ex.Reader = bufio.NewReader(strings.NewReader("n\n"))

	marker := filepath.Join(ex.WorkspaceDir, "cancel-marker.txt")
	out, err := ex.Execute(types.ActionBash, "touch cancel-marker.txt")
	if err != nil {
		t.Fatalf("user cancellation should not be an error, got: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "dibatalkan") {
		t.Errorf("expected user cancellation message, got %q", out)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Error("command must not run after user cancels")
	}
}

func TestExecuteBash_CopilotConfirmRuns(t *testing.T) {
	ex, _ := newTestExecutor(t)
	ex.SecurityMode = types.ModeCopilot
	ex.Reader = bufio.NewReader(strings.NewReader("y\n"))

	out, err := ex.Execute(types.ActionBash, "echo dijalankan")
	if err != nil {
		t.Fatalf("confirmed command failed: %v", err)
	}
	if !strings.Contains(out, "dijalankan") {
		t.Errorf("expected command output, got %q", out)
	}
}

func TestExecuteBash_Timeout(t *testing.T) {
	ex, _ := newTestExecutor(t)
	ex.Timeout = 100 * time.Millisecond

	start := time.Now()
	_, err := ex.Execute(types.ActionBash, "sleep 5")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context deadline exceeded, got %v", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("bash timeout did not stop process in time: %s", elapsed)
	}
}

func TestExecuteRejectsUnknownAction(t *testing.T) {
	ex, _ := newTestExecutor(t)
	if _, err := ex.Execute("report_done", "selesai"); err == nil {
		t.Error("expected unknown action to return an error")
	}
}

func TestValidateWorkspacePathRejectsSymlinkEscape(t *testing.T) {
	ex, workspace := newTestExecutor(t)
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("SECRET"), 0644); err != nil {
		t.Fatalf("failed to write secret: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "linkdir")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if err := os.Symlink(secret, filepath.Join(workspace, "linkfile")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if _, err := ex.Execute(types.ActionRead, "linkdir/secret.txt"); err == nil {
		t.Error("read through a symlinked directory must be rejected")
	}
	if _, err := ex.Execute(types.ActionRead, "linkfile"); err == nil {
		t.Error("read through a symlinked file must be rejected")
	}
	if _, err := ex.Execute(types.ActionWrite, "linkdir/new.txt\nPWNED"); err == nil {
		t.Error("write through a symlinked directory must be rejected")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "new.txt")); !os.IsNotExist(statErr) {
		t.Error("write through a symlink must not create a file outside the workspace")
	}

	// A symlink that stays inside the workspace is still allowed.
	inside := filepath.Join(workspace, "inside.txt")
	if err := os.WriteFile(inside, []byte("INSIDE"), 0644); err != nil {
		t.Fatalf("failed to write inside file: %v", err)
	}
	if err := os.Symlink(inside, filepath.Join(workspace, "alias.txt")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if out, err := ex.Execute(types.ActionRead, "alias.txt"); err != nil || out != "INSIDE" {
		t.Errorf("symlink inside workspace should be allowed, out=%q err=%v", out, err)
	}
}
