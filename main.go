package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"omyang/internal/config"
	"omyang/internal/engine"
	"omyang/internal/repl"
	"omyang/internal/types"
)

// engineFactory is replaceable in tests to avoid starting llama-server or
// calling the cloud API.
var engineFactory = buildConfiguredEngine

func main() {
	os.Exit(execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, resolveBaseDir()))
}

func execute(args []string, stdin io.Reader, stdout, stderr io.Writer, baseDir string) int {
	fs := flag.NewFlagSet("omyang", flag.ContinueOnError)
	fs.SetOutput(stderr)
	engineFlag := fs.String("engine", "", "override engine: local|cloud")
	modeFlag := fs.String("mode", "", "override session mode: talk|plan|work (atau auto|copilot)")
	modelFlag := fs.String("model", "", "path model .gguf lokal")
	roleFlag := fs.String("role", "", "nama role dari roles.json")
	skillFlag := fs.String("skill", "", "nama skill dari ./skills/")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	prompt := strings.Join(fs.Args(), " ")
	if stdinIsPipe(stdin) {
		data, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "Gagal membaca stdin: %v\n", err)
			return 1
		}
		piped := strings.TrimSpace(string(data))
		if piped != "" {
			if prompt != "" {
				prompt = prompt + "\n\n" + piped
			} else {
				prompt = piped
			}
		}
	}
	stateless := prompt != ""

	opts := config.CLIOptions{
		EngineOverride: *engineFlag,
		ModeOverride:   *modeFlag,
		ModelOverride:  *modelFlag,
		RoleOverride:   *roleFlag,
		SkillOverride:  *skillFlag,
		InitialPrompt:  prompt,
	}
	cfg, warnings, err := config.LoadConfig(baseDir, opts)
	if err != nil {
		fmt.Fprintf(stderr, "Gagal memuat konfigurasi: %v\n", err)
		return 1
	}
	for _, warning := range warnings {
		fmt.Fprintf(stderr, "Peringatan: %s\n", warning)
	}

	roles, roleErr := config.LoadRoles(cfg.RolesPath)
	if roleErr != nil {
		fmt.Fprintf(stderr, "Peringatan: %v\n", roleErr)
	}

	activeEngine, err := engineFactory(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "Gagal menyiapkan engine: %v\n", err)
		return 1
	}

	factory := func(target string) (engine.Engine, error) {
		// Build from a scoped copy: a failed switch must not mutate live config.
		scoped := *cfg
		scoped.Engine.ActiveEngine = target
		return engineFactory(&scoped)
	}
	session := repl.NewSession(cfg, roles, activeEngine, factory, stdin, stdout)
	defer session.Close()
	stopSignals := handleSignals(session, stdout)
	defer stopSignals()
	session.SetStateless(stateless)

	if *skillFlag != "" {
		session.HandleCommand("/skill " + *skillFlag)
	}

	if stateless {
		if *modeFlag == "" {
			session.HandleCommand("/work")
		}
		result, err := session.RunTurn(context.Background(), prompt)
		if err != nil {
			fmt.Fprintf(stderr, "Gagal menyelesaikan tugas: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, result)
		return 0
	}

	for {
		fmt.Fprintln(stdout, session.FormatStatusBar())
		fmt.Fprint(stdout, "> ")
		line, err := session.ReadLine()
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(stdout)
				return 0
			}
			fmt.Fprintf(stderr, "Gagal membaca input: %v\n", err)
			return 1
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		handled, exit := session.HandleCommand(line)
		if exit {
			return 0
		}
		if handled {
			continue
		}

		result, err := session.RunTurn(context.Background(), line)
		if err != nil {
			fmt.Fprintf(stderr, "Gagal menyelesaikan giliran: %v\n", err)
			continue
		}
		fmt.Fprintln(stdout, result)
	}
}

// handleSignals closes the engine on Ctrl-C/SIGTERM so llama-server is not orphaned.
// The returned function stops the handler without exiting.
func handleSignals(session *repl.Session, out io.Writer) func() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-signals:
			_ = session.Close()
			fmt.Fprintln(out)
			os.Exit(130)
		case <-done:
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}

func stdinIsPipe(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}

func resolveBaseDir() string {
	if executable, err := os.Executable(); err == nil {
		// go run/go test binaries live in a temporary build directory; fall back
		// to the working directory for those cases.
		if !strings.Contains(executable, string(filepath.Separator)+"go-build") {
			return filepath.Dir(executable)
		}
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return workingDir
}

func buildConfiguredEngine(cfg *config.Config) (engine.Engine, error) {
	if cfg.Engine.ActiveEngine == types.EngineCloud {
		if err := config.ValidateCloudKey(cfg); err != nil {
			return nil, err
		}
		return engine.NewCloudEngine(cfg), nil
	}

	modelPath, err := config.DiscoverLocalModel(cfg.ModelsDir, cfg.Engine.LocalModelPath)
	if err != nil {
		return nil, err
	}
	cfg.Engine.LocalModelPath = modelPath

	local := engine.NewLocalEngine(cfg)
	if err := local.Start(); err != nil {
		return nil, err
	}
	return local, nil
}
