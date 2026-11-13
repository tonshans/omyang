package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omyang/internal/config"
	"omyang/internal/engine"
	"omyang/internal/types"
)

type fakeEngine struct {
	reply string
	calls [][]types.Message
}

func (f *fakeEngine) Name() string { return "fake" }

func (f *fakeEngine) Close() error { return nil }

func (f *fakeEngine) Generate(_ context.Context, messages []types.Message, allowedActions []string) (*types.AgentResponse, error) {
	f.calls = append(f.calls, append([]types.Message(nil), messages...))
	return &types.AgentResponse{
		Thought: "Jawaban akhir dari mesin palsu yang cukup panjang.",
		Action:  types.ActionFinalAnswer,
		Input:   f.reply,
	}, nil
}

func withFakeEngine(t *testing.T, fake *fakeEngine) {
	t.Helper()
	original := engineFactory
	engineFactory = func(cfg *config.Config) (engine.Engine, error) {
		return fake, nil
	}
	t.Cleanup(func() { engineFactory = original })
}

func assertNoSessionState(t *testing.T, baseDir string) {
	t.Helper()
	statePath := filepath.Join(baseDir, ".session_state.json")
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("mode stateless tidak boleh menulis %s, err=%v", statePath, err)
	}
}

func TestExecuteStatelessPrompt(t *testing.T) {
	baseDir := t.TempDir()
	fake := &fakeEngine{reply: "HASIL_ONE_OFF"}
	withFakeEngine(t, fake)

	var stdout, stderr bytes.Buffer
	code := execute([]string{"periksa", "kapasitas", "disk"}, strings.NewReader(""), &stdout, &stderr, baseDir)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "HASIL_ONE_OFF") {
		t.Errorf("stdout harus memuat jawaban akhir, dapat: %q", stdout.String())
	}
	if len(fake.calls) != 1 {
		t.Fatalf("engine harus dipanggil 1 kali, dapat %d", len(fake.calls))
	}
	if !strings.Contains(messagesText(fake.calls[0]), "periksa kapasitas disk") {
		t.Error("prompt CLI harus diteruskan ke engine")
	}
	assertNoSessionState(t, baseDir)
}

func TestExecuteStatelessUnixPipe(t *testing.T) {
	baseDir := t.TempDir()
	fake := &fakeEngine{reply: "HASIL_PIPE"}
	withFakeEngine(t, fake)

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("gagal membuat pipe: %v", err)
	}
	if _, err := writer.WriteString("BARIS_DARI_PIPE"); err != nil {
		t.Fatalf("gagal menulis pipe: %v", err)
	}
	_ = writer.Close()
	defer reader.Close()

	var stdout, stderr bytes.Buffer
	code := execute([]string{"analisa", "error", "ini"}, reader, &stdout, &stderr, baseDir)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "HASIL_PIPE") {
		t.Errorf("stdout harus memuat jawaban akhir, dapat: %q", stdout.String())
	}
	if len(fake.calls) != 1 {
		t.Fatalf("engine harus dipanggil 1 kali, dapat %d", len(fake.calls))
	}
	prompt := messagesText(fake.calls[0])
	if !strings.Contains(prompt, "analisa error ini") || !strings.Contains(prompt, "BARIS_DARI_PIPE") {
		t.Errorf("prompt harus menggabungkan argumen CLI dan isi pipe: %s", prompt)
	}
	assertNoSessionState(t, baseDir)
}

func TestExecuteInteractiveExits(t *testing.T) {
	baseDir := t.TempDir()
	fake := &fakeEngine{reply: "TIDAK_DIPAKAI"}
	withFakeEngine(t, fake)

	var stdout, stderr bytes.Buffer
	code := execute(nil, strings.NewReader("/exit\n"), &stdout, &stderr, baseDir)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "💻 Local") {
		t.Errorf("mode interaktif harus menampilkan status bar, dapat: %q", stdout.String())
	}
	if len(fake.calls) != 0 {
		t.Errorf("/exit tidak boleh memanggil engine, calls=%d", len(fake.calls))
	}
}

func messagesText(messages []types.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(msg.Content)
		b.WriteString("\n")
	}
	return b.String()
}
