package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/jgabor/aila/internal/app"
	"github.com/jgabor/aila/internal/diagnostic"
	historypkg "github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/state"
)

var cachedTestBinary string

func TestMain(m *testing.M) {
	binary, err := buildAilaTestBinaryOnce()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build aila test binary for smoke tests: %v\n", err)
		os.Exit(1)
	}
	cachedTestBinary = binary
	defer func() {
		_ = os.RemoveAll(filepath.Dir(cachedTestBinary))
	}()

	os.Exit(m.Run())
}

func buildAilaTestBinaryOnce() (string, error) {
	tmp, err := os.MkdirTemp("", "aila-test-build-*")
	if err != nil {
		return "", err
	}
	binary := filepath.Join(tmp, "aila")
	build := exec.Command("go", "build", "-o", binary, ".")
	// Use absolute path for build source to be safe.
	wd, _ := os.Getwd()
	build.Dir = wd
	if output, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build aila test binary: %v\n%s", err, output)
	}
	return binary, nil
}

func TestStaticTUISmokeStartupAndQuit(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	ctx, cancel, terminal, wait := startAilaPTY(t)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	startup := readUntil(t, terminal, "Aila", 20*time.Second)
	if !strings.Contains(startup, "Aila") {
		t.Fatalf("startup output missing Aila marker: %q", startup)
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input: %v", err)
	}

	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("static TUI quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("static TUI did not quit after q: %v", ctx.Err())
	}
}

func TestM15ProjectStoreStartupPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithSizeEnvAndWorkspace(t, 80, 24, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	startup := readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
	}, 20*time.Second)
	for _, forbidden := range []string{workspace, env.home, env.xdgConfigHome, "/tmp", "/home/", ".aila", "project.toml", "artifacts/", "indexes/"} {
		if strings.Contains(startup, forbidden) {
			t.Fatalf("startup store smoke leaked path marker %q: %q", forbidden, startup)
		}
	}

	assertProjectStoreLayout(t, workspace)
	if _, err := os.Stat(filepath.Join(env.home, ".aila")); !os.IsNotExist(err) {
		t.Fatalf("PTY startup touched HOME project store path, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(env.home, ".config", "aila", ".aila")); !os.IsNotExist(err) {
		t.Fatalf("PTY startup touched HOME config project store path, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(mustSourceDir(t), ".aila")); !os.IsNotExist(err) {
		t.Fatalf("PTY startup touched source package project store path, err=%v", err)
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("project store startup TUI quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("project store startup TUI did not quit after q: %v", ctx.Err())
	}
}

func TestM15AShutdownDiagnosticsPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	ctx, cancel, terminal, wait, workspace, cmd := startAilaPTYWithProcess(t, 80, 24, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	startup := readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
	}, 20*time.Second)
	assertNoDiagnosticSmokeLeaks(t, startup, env, workspace)
	assertProjectStoreLayout(t, workspace)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM to PTY process: %v", err)
	}
	shutdown := readUntilAll(t, terminal, []string{
		"shutdown:",
		"signal_shutdown",
		"signal-triggered shutdown requested",
	}, 10*time.Second)

	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("shutdown PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("shutdown PTY did not clean up before timeout: %v", ctx.Err())
	}
	shutdown += readRemainingPTYOutput(t, terminal, 2*time.Second)
	assertNoDiagnosticSmokeLeaks(t, shutdown, env, workspace)

	assertProjectStoreLayout(t, workspace)
	assertNoDurableStatePollution(t, env, baseline)
	if _, err := os.Stat(filepath.Join(env.xdgConfigHome, "aila", "config.toml")); err != nil {
		t.Fatalf("temp XDG config was not created for shutdown smoke: %v", err)
	}
}

func TestM15ADebugDiagnosticsNonInteractiveSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("process smoke uses Unix path assertions")
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	storeRoot := filepath.Join(workspace, ".aila")
	if err := os.MkdirAll(storeRoot, 0o755); err != nil {
		t.Fatalf("create debug smoke project store: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storeRoot, "project.toml"), []byte("schema_version = 2\nsecret = \"token=debug-smoke-secret\"\npath = \""+workspace+"\"\n"), 0o644); err != nil {
		t.Fatalf("write debug smoke corrupt metadata: %v", err)
	}
	binary := buildAilaTestBinary(t, env.vars, tmp)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--debug")
	cmd.Dir = workspace
	cmd.Env = append(env.vars,
		"OPENAI_API_KEY=debug-smoke-openai-secret",
		"AILA_TEST_CONFIG_PATH="+filepath.Join(env.xdgConfigHome, "aila", "config.toml"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run debug diagnostics smoke: %v\n%s", err, output)
	}
	if ctx.Err() != nil {
		t.Fatalf("debug diagnostics smoke exceeded timeout: %v", ctx.Err())
	}
	encoded := string(output)
	if len(encoded) > app.MaxDebugDiagnosticOutputBytes {
		t.Fatalf("debug diagnostics output length = %d, want <= %d", len(encoded), app.MaxDebugDiagnosticOutputBytes)
	}
	for _, marker := range []string{"\"diagnostics\"", "\"count\"", "\"max_count\"", "\"max_message_bytes\"", "\"max_output_bytes\""} {
		if !strings.Contains(encoded, marker) {
			t.Fatalf("debug diagnostics output missing structured marker %q: %s", marker, encoded)
		}
	}
	assertNoDiagnosticSmokeLeaks(t, encoded, env, workspace)
	for _, leaked := range []string{"debug-smoke-secret", "debug-smoke-openai-secret", "token=", "OPENAI_API_KEY", "config.toml"} {
		if strings.Contains(encoded, leaked) {
			t.Fatalf("debug diagnostics smoke leaked %q: %s", leaked, encoded)
		}
	}

	var decoded app.DebugDiagnosticsOutput
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("unmarshal debug diagnostics smoke output: %v\n%s", err, output)
	}
	if decoded.Count != 1 || len(decoded.Diagnostics) != 1 || decoded.MaxCount != app.MaxDebugDiagnostics || decoded.MaxOutputBytes != app.MaxDebugDiagnosticOutputBytes {
		t.Fatalf("debug diagnostics smoke bounds = %+v", decoded)
	}
	got := decoded.Diagnostics[0]
	if got.Source != "state.open" || got.AffectedArtifact != "project_metadata" || got.RecoveryAction != "manual_repair" || !got.UserInputNeeded {
		t.Fatalf("debug diagnostics smoke diagnostic = %+v", got)
	}

	assertProjectStoreEntries(t, workspace)
	assertFileContent(t, filepath.Join(storeRoot, "project.toml"), "schema_version = 2\nsecret = \"token=debug-smoke-secret\"\npath = \""+workspace+"\"\n")
	assertNoDurableStatePollution(t, env, baseline)
	if _, err := os.Stat(filepath.Join(env.xdgConfigHome, "aila", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("debug smoke unexpectedly created temp config file, err=%v", err)
	}
}

func TestM16ContinueResumePTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	for _, args := range [][]string{{"continue"}, {"--continue"}} {
		args := args
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env := newAilaPTYEnv(t)
			baseline := captureDurableStateBaseline(t)
			ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, args, 160, 60, env.vars, func(workspace string) {
				seedCurrentSessionSnapshot(t, workspace)
			})
			defer cancel()
			defer func() { _ = terminal.Close() }()

			output := readUntilAll(t, terminal, []string{
				"Aila",
				"Runtime idle",
				"Runtime status:",
				"status source: runtime.dispatch",
				"detail: resumed current session",
				"result: remembered smoke result",
				"Resumed memory:",
				"source: state.current-session-snapshot",
				"session id: current",
				"resumed transcript turns: 2",
				"queued count: 1",
				"diagnostics: 1",
				"blocker: remembered smoke blocker",
				"concern: remembered smoke concern",
				"Queued input:",
				"queued: remembered queued smoke input",
				"user: remembered smoke prompt",
				"assistant: remembered smoke answer",
				"Diagnostics:",
				"message: remembered smoke diagnostic",
			}, 20*time.Second)
			assertNoResumeSmokeLeaks(t, output, env, workspace)
			assertCurrentSessionSnapshotState(t, workspace)

			if _, err := terminal.Write([]byte("q")); err != nil {
				t.Fatalf("send q quit input after resume smoke: %v", err)
			}
			select {
			case err := <-wait:
				if err != nil {
					t.Fatalf("resume PTY returned error: %v", err)
				}
			case <-ctx.Done():
				t.Fatalf("resume PTY did not clean up before timeout: %v", ctx.Err())
			}

			assertCurrentSessionSnapshotState(t, workspace)
			assertNoDurableStatePollution(t, env, baseline)
		})
	}
}

func TestM16ContinueNoMemoryPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	for _, args := range [][]string{{"continue"}, {"--continue"}} {
		args := args
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env := newAilaPTYEnv(t)
			baseline := captureDurableStateBaseline(t)
			ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, args, 80, 24, env.vars, nil)
			defer cancel()
			defer func() { _ = terminal.Close() }()

			output := readUntilAll(t, terminal, []string{
				"Aila",
				"Stage IDLE",
				"project store: initialized - project store ready",
				"Prompt",
				">",
			}, 20*time.Second)
			for _, forbidden := range []string{"Resumed memory:", "state.current-session-snapshot", "remembered smoke", "Queued input:"} {
				if strings.Contains(output, forbidden) {
					t.Fatalf("no-memory continue output exposed memory marker %q: %q", forbidden, output)
				}
			}
			assertNoResumeSmokeLeaks(t, output, env, workspace)
			assertProjectStoreLayout(t, workspace)

			if _, err := terminal.Write([]byte("q")); err != nil {
				t.Fatalf("send q quit input after no-memory continue smoke: %v", err)
			}
			select {
			case err := <-wait:
				if err != nil {
					t.Fatalf("no-memory continue PTY returned error: %v", err)
				}
			case <-ctx.Done():
				t.Fatalf("no-memory continue PTY did not clean up before timeout: %v", ctx.Err())
			}

			assertProjectStoreLayout(t, workspace)
			assertNoDurableStatePollution(t, env, baseline)
		})
	}
}

func TestNonInteractiveRunThenContinueSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("process and PTY smoke uses Unix path assertions")
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	seedNonInteractiveRunSmokeWorkspace(t, workspace)
	binary := buildAilaTestBinary(t, env.vars, tmp)

	runCtx, runCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer runCancel()
	runCmd := exec.CommandContext(runCtx, binary, "run", "explain", "the", "repo")
	runCmd.Dir = workspace
	runCmd.Env = append(env.vars, "OPENAI_API_KEY=run-smoke-secret")
	runOutputBytes, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("non-interactive run smoke failed: %v\n%s", err, runOutputBytes)
	}
	if runCtx.Err() != nil {
		t.Fatalf("non-interactive run smoke exceeded timeout: %v", runCtx.Err())
	}
	runOutput := string(runOutputBytes)
	for _, want := range []string{
		"command: run",
		"mode: non_interactive_read_only",
		"status: flagged",
		"prompt: explain the repo",
		"inspected_files:",
		"README.md status=completed",
		"ROADMAP.md status=completed",
		"commands_run:",
		"git status --short --branch status=completed",
		"git diff --stat status=completed",
		"caveats:",
		"provider model execution deferred",
		"source_refs:",
		"stored_session: true",
		"stored_history: true",
	} {
		if !strings.Contains(runOutput, want) {
			t.Fatalf("run smoke output missing %q:\n%s", want, runOutput)
		}
	}
	assertNoNonInteractiveRunSmokeLeaks(t, runOutput, env, workspace)
	assertNonInteractiveRunStoreState(t, workspace)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "continue")
	cmd.Dir = workspace
	cmd.Env = env.vars
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 60, Cols: 160})
	if err != nil {
		t.Fatalf("start continue after run smoke PTY: %v", err)
	}
	defer func() { _ = terminal.Close() }()
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()

	resumeOutput := readUntilAll(t, terminal, []string{
		"Aila",
		"Resumed memory:",
		"source: state.current-session-snapshot",
		"run mode: non_interactive_read_only",
		"run status: flagged",
		"run prompt: explain the repo",
		"inspected file: README.md status=completed",
		"command run: git status --short --branch status=completed",
		"run caveat: deterministic read-only run; provider model execution deferred",
		"source ref: git diff --stat",
	}, 20*time.Second)
	assertNoNonInteractiveRunSmokeLeaks(t, resumeOutput, env, workspace)

	if _, err := terminal.Write([]byte("/history\r")); err != nil {
		t.Fatalf("send /history after run smoke: %v", err)
	}
	historyOutput := readUntilAll(t, terminal, []string{
		"history:",
		"read-only: true",
		"entries: 5",
		"noninteractive-run current noninteractive-run-1 prompt noninteractive run prompt explain the repo",
		"noninteractive-run current noninteractive-run-2 response Read-only run flagged",
		"noninteractive-run current noninteractive-run-4 command check git status --short --branch completed",
		"selected event id: noninteractive-run-1",
		"selected run id: noninteractive-run",
		"selected session id: current",
	}, 10*time.Second)
	assertNoNonInteractiveRunSmokeLeaks(t, historyOutput, env, workspace)

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after run continue smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("run continue PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("run continue PTY did not clean up before timeout: %v", ctx.Err())
	}

	assertNonInteractiveRunStoreState(t, workspace)
	assertNoDurableStatePollution(t, env, baseline)
}

func TestNonInteractiveWriteRunThenContinueSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("process and PTY smoke uses Unix path assertions")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	seedNonInteractiveWriteRunSmokeWorkspace(t, workspace)
	binary := buildAilaTestBinary(t, env.vars, tmp)

	runCtx, runCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer runCancel()
	runCmd := exec.CommandContext(runCtx, binary, "run", "create", "a", "note")
	runCmd.Dir = workspace
	runCmd.Env = append(env.vars, "OPENAI_API_KEY=write-run-smoke-secret")
	runOutputBytes, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("non-interactive write run smoke failed: %v\n%s", err, runOutputBytes)
	}
	if runCtx.Err() != nil {
		t.Fatalf("non-interactive write run smoke exceeded timeout: %v", runCtx.Err())
	}
	runOutput := string(runOutputBytes)
	for _, want := range []string{
		"command: run",
		"mode: non_interactive_write",
		"status: flagged",
		"prompt: create a note",
		"changed_files:",
		"docs/aila-run-output.md status=completed",
		"mutation:",
		"tool=write status=completed path=docs/aila-run-output.md",
		"decision_source=autonomy_policy autonomy=yolo allowed=true automatic=true approval_required=false",
		"commands_run:",
		"git status --short --branch status=completed",
		"git diff --stat status=completed",
		"blockers:",
		"- none",
		"caveats:",
		"provider model execution deferred",
		"source_refs:",
		"docs/aila-run-output.md",
		"stored_session: true",
		"stored_history: true",
	} {
		if !strings.Contains(runOutput, want) {
			t.Fatalf("write run smoke output missing %q:\n%s", want, runOutput)
		}
	}
	assertNoNonInteractiveRunSmokeLeaks(t, runOutput, env, workspace)
	written, err := os.ReadFile(filepath.Join(workspace, "docs", "aila-run-output.md"))
	if err != nil {
		t.Fatalf("read non-interactive write target: %v", err)
	}
	if !strings.Contains(string(written), "Prompt: create a note") {
		t.Fatalf("write target content = %q, want prompt evidence", written)
	}
	assertNonInteractiveWriteRunStoreState(t, workspace)

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "continue")
	cmd.Dir = workspace
	cmd.Env = env.vars
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 90, Cols: 160})
	if err != nil {
		t.Fatalf("start continue after write run smoke PTY: %v", err)
	}
	defer func() { _ = terminal.Close() }()
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()

	resumeOutput := readUntilAll(t, terminal, []string{
		"Aila",
		"Resumed memory:",
		"run mode: non_interactive_write",
		"run status: flagged",
		"run prompt: create a note",
		"changed file: docs/aila-run-output.md status=completed",
		"mutation tool: write",
		"mutation status: completed",
		"mutation decision source: autonomy_policy",
		"mutation decision autonomy: yolo",
		"mutation approval required: false",
	}, 20*time.Second)
	assertNoNonInteractiveRunSmokeLeaks(t, resumeOutput, env, workspace)

	if _, err := terminal.Write([]byte("/history\r")); err != nil {
		t.Fatalf("send /history after write run smoke: %v", err)
	}
	historyOutput := readUntilAll(t, terminal, []string{
		"history:",
		"read-only: true",
		"entries: 6",
		"noninteractive-run current noninteractive-run-6 mutation write completed docs/aila-run-output.md",
		"selected event id: noninteractive-run-1",
	}, 10*time.Second)
	assertNoNonInteractiveRunSmokeLeaks(t, historyOutput, env, workspace)

	if _, err := terminal.Write([]byte("\x1b[B\x1b[B\x1b[B\x1b[B\x1b[B")); err != nil {
		t.Fatalf("send write history down navigation input: %v", err)
	}
	mutationOutput := readUntilAll(t, terminal, []string{
		"selected: 6",
		"selected event id: noninteractive-run-6",
		"selected kind: mutation",
		"selected changed paths: docs/aila-run-output.md",
		"selected undo available: true",
		"selected undo action: delete_created_file",
	}, 10*time.Second)
	assertNoNonInteractiveRunSmokeLeaks(t, mutationOutput, env, workspace)

	if _, err := terminal.Write([]byte{0x18, 'd'}); err != nil {
		t.Fatalf("send diff shortcut after write history smoke: %v", err)
	}
	diffOutput := readUntilAll(t, terminal, []string{
		"read_only: true",
		"source: git diff",
		"status: ready",
		"file: docs/aila-run-output.md",
		"file_status: added",
		"line_addition: # Aila Non-Interactive",
		"line_addition: Prompt: create a note",
	}, 10*time.Second)
	assertNoNonInteractiveRunSmokeLeaks(t, diffOutput, env, workspace)

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after write run smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("write run continue PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("write run continue PTY did not clean up before timeout: %v", ctx.Err())
	}

	assertNonInteractiveWriteRunStoreState(t, workspace)
	assertNoDurableStatePollution(t, env, baseline)
}

func TestHistoryViewPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 120, 45, env.vars, func(workspace string) {
		seedFakeHistoryEvents(t, workspace)
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)
	if _, err := terminal.Write([]byte("/history\r")); err != nil {
		t.Fatalf("send /history command input: %v", err)
	}

	output := readUntilAll(t, terminal, []string{
		"history:",
		"read-only: true",
		"entries: 5",
		"selected: 1",
		"history-run history-session history-event-1 prompt user asked for fake history",
		"history-run history-session history-event-2 response fake response summary",
		"history-run history-session history-event-3 command history command summary",
		"history-run history-session history-event-4 runtime runtime idle: smoke complete",
		"history-run history-session history-event-5 mutation write completed notes.txt",
		"selected event id: history-event-1",
		"selected run id: history-run",
		"selected session id: history-session",
		"selected kind: prompt",
		"selected text: user asked for fake history",
	}, 10*time.Second)
	assertNoHistorySmokeLeaks(t, output, env, workspace)
	for _, forbidden := range []string{"replay", "token=", "Authorization:", "history-smoke-secret"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("history PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

	if _, err := terminal.Write([]byte("\x1b[B\x1b[B\x1b[B\x1b[B")); err != nil {
		t.Fatalf("send history down navigation input: %v", err)
	}
	mutationOutput := readUntilAll(t, terminal, []string{
		"selected: 5",
		"selected event id: history-event-5",
		"selected kind: mutation",
		"selected changed paths: notes.txt",
		"selected approval id: smoke-approval",
		"selected undo available: true",
		"selected undo action: delete_created_file",
	}, 10*time.Second)
	assertNoHistorySmokeLeaks(t, mutationOutput, env, workspace)

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after history smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("history PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("history PTY did not clean up before timeout: %v", ctx.Err())
	}

	assertFakeHistoryProjectStoreState(t, workspace)
	assertNoDurableStatePollution(t, env, baseline)
	if _, err := os.Stat(filepath.Join(env.xdgConfigHome, "aila", "config.toml")); err != nil {
		t.Fatalf("temp XDG config was not created for history smoke: %v", err)
	}
}

func TestHistoryEmptyPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithSizeEnvAndWorkspace(t, 80, 24, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)
	if _, err := terminal.Write([]byte{0x18, 'h'}); err != nil {
		t.Fatalf("send ctrl+x h history shortcut input: %v", err)
	}

	output := readUntilAll(t, terminal, []string{
		"history:",
		"read-only: true",
		"empty history",
		"no fake history events recorded yet",
	}, 10*time.Second)
	assertNoHistorySmokeLeaks(t, output, env, workspace)
	for _, forbidden := range []string{"entries:", "selected event id:", "replay"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("empty history PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after empty history smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("empty history PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("empty history PTY did not clean up before timeout: %v", ctx.Err())
	}

	assertProjectStoreLayout(t, workspace)
	assertNoDurableStatePollution(t, env, baseline)
	if _, err := os.Stat(filepath.Join(env.xdgConfigHome, "aila", "config.toml")); err != nil {
		t.Fatalf("temp XDG config was not created for empty history smoke: %v", err)
	}
}

func TestDiffViewPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 120, 32, env.vars, func(workspace string) {
		seedDiffSmokeWorkspace(t, workspace)
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)
	if _, err := terminal.Write([]byte("/diff\r")); err != nil {
		t.Fatalf("send /diff command input: %v", err)
	}

	output := readUntilAll(t, terminal, []string{
		"diff:",
		"read-only: true",
		"source: git diff",
		"status: ready",
		"file: internal/demo.txt status: modified",
		"- old value",
		"+ new value",
	}, 10*time.Second)
	assertNoDiffSmokeLeaks(t, output, env, workspace)
	for _, forbidden := range []string{"replay", "Mutation result:", "Approval pending:", "provider review"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("diff PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

	if _, err := terminal.Write([]byte{0x1b}); err != nil {
		t.Fatalf("send Escape after diff smoke: %v", err)
	}
	closed := readUntilAll(t, terminal, []string{"Display labels:", "No messages yet."}, 10*time.Second)
	if strings.Contains(closed, "diff:") {
		t.Fatalf("diff PTY escape did not leave diff view: %q", closed)
	}
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after diff smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("diff PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("diff PTY did not clean up before timeout: %v", ctx.Err())
	}

	assertProjectStoreLayout(t, workspace)
	assertNoDurableStatePollution(t, env, baseline)
}

func TestPromptSubmitPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	ctx, cancel, terminal, wait, _ := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 160, 45, env.vars, func(workspace string) {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nAila is a bounded read-only coding agent.\n"), 0o644); err != nil {
			t.Fatalf("seed README for prompt submit PTY smoke: %v", err)
		}
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntil(t, terminal, "Aila", 20*time.Second)
	if _, err := terminal.Write([]byte("explain this repo\r")); err != nil {
		t.Fatalf("send prompt submit input: %v", err)
	}

	output := readUntilAll(t, terminal, []string{
		"Stage BUILD | Runtime idle",
		"Runtime status:",
		"status source: runtime.dispatch",
		"detail: read tool dispatch",
		"active: false",
		"result: I will inspect README.md before answering. Read-only inspection completed.",
		"user: explain this repo",
		"assistant: I will inspect README.md before answering. Read-only inspection completed.",
		"Read tool:",
		"status: completed",
		"read-only: true",
		"path: README.md",
	}, 10*time.Second)
	for _, forbidden := range []string{"OPENAI", "ANTHROPIC", "GOOGLE_API", "credential", "write class", "approval prompt", "config.toml", ".config/aila"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("runtime prompt PTY exposed provider/tool/config marker %q: %q", forbidden, output)
		}
	}
	if _, err := os.Stat(filepath.Join(env.xdgConfigHome, "aila", "config.toml")); err != nil {
		t.Fatalf("temp XDG config was not created for scrubbed PTY startup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.home, ".config", "aila")); !os.IsNotExist(err) {
		t.Fatalf("scrubbed PTY touched HOME config path, err=%v", err)
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("prompt submit TUI quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("prompt submit TUI did not quit after q: %v", ctx.Err())
	}
}

func TestM25ApprovalDecisionPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	for _, tc := range []struct {
		name   string
		input  string
		result string
	}{
		{name: "approve", input: "a", result: "approval approve: internal/demo.txt"},
		{name: "deny", input: "n", result: "approval deny: internal/demo.txt"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newAilaPTYEnv(t)
			env.vars = append(env.vars, "AILA_FAKE_APPROVAL_PROPOSAL=1")
			ctx, cancel, terminal, wait, workspace := startAilaPTYWithSizeEnvAndWorkspace(t, 160, 45, env.vars)
			defer cancel()
			defer func() { _ = terminal.Close() }()

			startup := readUntilAll(t, terminal, []string{
				"Approval pending:",
				"proposal id: fake-approval-001",
				"path: internal/demo.txt",
				"command: write internal/demo.txt",
				"choices: a approve | n deny | d defer",
				"mutation executed: false",
			}, 20*time.Second)
			if strings.Contains(startup, "mutation executed: true") {
				t.Fatalf("M25 approval startup implied mutation execution: %q", startup)
			}
			if _, err := terminal.Write([]byte(tc.input)); err != nil {
				t.Fatalf("send M25 approval input: %v", err)
			}

			output := readUntilAll(t, terminal, []string{
				"Runtime idle",
				tc.result,
			}, 10*time.Second)
			if strings.Contains(output, "mutation executed: true") {
				t.Fatalf("M25 approval decision implied mutation execution: %q", output)
			}
			if _, err := os.Stat(filepath.Join(workspace, "internal", "demo.txt")); !os.IsNotExist(err) {
				t.Fatalf("approval PTY wrote fake target, err=%v", err)
			}
			if _, err := terminal.Write([]byte("q")); err != nil {
				t.Fatalf("send q after M25 approval: %v", err)
			}
			select {
			case err := <-wait:
				if err != nil {
					t.Fatalf("M25 approval PTY quit returned error: %v", err)
				}
			case <-ctx.Done():
				t.Fatalf("M25 approval PTY did not quit cleanly: %v", ctx.Err())
			}
		})
	}
}

func TestApprovalToWriteMutationPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	env.vars = append(env.vars,
		"AILA_FAKE_APPROVAL_WRITE=1",
		"AILA_FAKE_APPROVAL_WRITE_PATH=internal/fake-approval-write.txt",
		"AILA_FAKE_APPROVAL_WRITE_CONTENT=approved through pty\n",
	)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithSizeEnvAndWorkspace(t, 160, 45, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	startup := readUntilAll(t, terminal, []string{
		"Approval pending:",
		"proposal id: fake-approval-write-001",
		"operation kind: mutation",
		"path: internal/fake-approval-write.txt",
		"choices: a approve | n deny | d defer",
	}, 20*time.Second)
	if strings.Contains(startup, "Mutation result:") {
		t.Fatalf("approval-to-write mutation startup ran mutation before approval: %q", startup)
	}
	if _, err := terminal.Write([]byte("a")); err != nil {
		t.Fatalf("send approval input: %v", err)
	}

	_ = readUntilAll(t, terminal, []string{
		"Runtime idle",
		"Mutation result:",
		"tool: write",
		"status: completed",
		"path: internal/fake-approval-write.txt",
		"decision source: autonomy_policy",
	}, 10*time.Second)
	if got, err := os.ReadFile(filepath.Join(workspace, "internal", "fake-approval-write.txt")); err != nil || string(got) != "approved through pty\n" {
		t.Fatalf("approval-to-write target content = %q err=%v", got, err)
	}
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after approval-to-write mutation: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("approval-to-write mutation PTY quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("approval-to-write mutation PTY did not quit cleanly: %v", ctx.Err())
	}
}

func TestUndoRedoRecoveryPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	env.vars = append(env.vars,
		"AILA_FAKE_APPROVAL_WRITE=1",
		"AILA_FAKE_APPROVAL_WRITE_PATH=notes.txt",
		"AILA_FAKE_APPROVAL_WRITE_CONTENT=approved through pty",
	)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithSizeEnvAndWorkspace(t, 160, 45, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Approval pending:",
		"proposal id: fake-approval-write-001",
		"operation kind: mutation",
		"path: notes.txt",
		"choices: a approve | n deny | d defer",
	}, 20*time.Second)
	if _, err := terminal.Write([]byte("a")); err != nil {
		t.Fatalf("send approval input before undo/redo smoke: %v", err)
	}

	readUntilAll(t, terminal, []string{
		"Runtime idle",
		"Mutation result:",
		"tool: write",
		"status: completed",
		"path: notes.txt",
	}, 10*time.Second)
	assertFileContent(t, filepath.Join(workspace, "notes.txt"), "approved through pty")

	if _, err := terminal.Write([]byte("/undo\r")); err != nil {
		t.Fatalf("send /undo command input: %v", err)
	}
	readUntilAll(t, terminal, []string{
		"Recovery result:",
		"command: undo",
		"status: completed",
		"action: delete_created_file",
		"paths: notes.txt",
		"redo available: true",
		"redo action: restore_created_file",
		"decision source: autonomy_policy",
	}, 10*time.Second)
	if _, err := os.Stat(filepath.Join(workspace, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("undo recovery target still exists or stat failed unexpectedly: %v", err)
	}

	if _, err := terminal.Write([]byte("/redo\r")); err != nil {
		t.Fatalf("send /redo command input: %v", err)
	}
	readUntilAll(t, terminal, []string{
		"command: redo",
		"status: completed",
		"action: restore_created_file",
		"redo available: false",
		"decision source: autonomy_policy",
		"decision tool: redo",
	}, 10*time.Second)
	assertFileContent(t, filepath.Join(workspace, "notes.txt"), "approved through pty")

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after undo/redo recovery smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("undo/redo recovery PTY quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("undo/redo recovery PTY did not quit cleanly: %v", ctx.Err())
	}

	assertUndoRedoRecoveryHistory(t, workspace)
}

func TestInteractiveReadOnlyBuildLoopPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	ctx, cancel, terminal, wait, _ := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 160, 45, env.vars, func(workspace string) {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nAila is a bounded read-only coding agent.\n"), 0o644); err != nil {
			t.Fatalf("seed README for read-only PTY smoke: %v", err)
		}
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntil(t, terminal, "Aila", 20*time.Second)
	if _, err := terminal.Write([]byte("summarize read only turn\r")); err != nil {
		t.Fatalf("send read-only prompt input: %v", err)
	}

	output := readUntilAll(t, terminal, []string{
		"Runtime idle",
		"result: I will inspect README.md before answering.",
		"Read tool:",
		"status: completed",
		"read-only: true",
		"path: README.md",
	}, 10*time.Second)
	if strings.Contains(output, "approval prompt") || strings.Contains(output, "write class") {
		t.Fatalf("read-only PTY smoke exposed out-of-scope approval/mutation text: %q", output)
	}
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after read-only prompt: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("read-only PTY quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("read-only PTY did not quit cleanly: %v", ctx.Err())
	}
}

func TestInteractiveWriteBuildLoopApprovalPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 160, 45, env.vars, func(workspace string) { seedInteractiveWriteBuildWorkspace(t, workspace) })
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{"Aila", "Prompt"}, 20*time.Second)
	if _, err := terminal.Write([]byte("create a note file for this workspace\r")); err != nil {
		t.Fatalf("send interactive write prompt input: %v", err)
	}

	approval := readUntilAll(t, terminal, []string{
		"Approval pending:",
		"proposal id: approval-call-write-1",
		"operation kind: mutation",
		"path: docs/interactive-build-output.md",
		"expected effect: create interactive build output file",
		"choices: a approve | n deny | d defer",
	}, 10*time.Second)
	if strings.Contains(approval, "Mutation result:") {
		t.Fatalf("interactive write ran mutation before approval: %q", approval)
	}
	if _, err := os.Stat(filepath.Join(workspace, "docs", "interactive-build-output.md")); !os.IsNotExist(err) {
		t.Fatalf("interactive write created file before approval: %v", err)
	}
	if _, err := terminal.Write([]byte("a")); err != nil {
		t.Fatalf("send interactive write approval input: %v", err)
	}

	readUntilAll(t, terminal, []string{
		"Runtime idle",
		"Mutation result:",
		"tool: write",
		"status: completed",
		"path: docs/interactive-build-output.md",
		"decision source: autonomy_policy",
		"approval required: false",
	}, 10*time.Second)
	content, err := os.ReadFile(filepath.Join(workspace, "docs", "interactive-build-output.md"))
	if err != nil || !strings.Contains(string(content), "create a note file for this workspace") {
		t.Fatalf("interactive write content = %q err=%v", content, err)
	}

	if _, err := terminal.Write([]byte("/history\r")); err != nil {
		t.Fatalf("send history command after interactive write: %v", err)
	}
	readUntilAll(t, terminal, []string{
		"history:",
		"read-only: true",
		"entries:",
		"undo enabled: true",
	}, 10*time.Second)
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after interactive write approval smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("interactive write approval PTY quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("interactive write approval PTY did not quit cleanly: %v", ctx.Err())
	}
}

func TestInteractiveWriteBuildLoopDenialPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 160, 45, env.vars, func(workspace string) { seedInteractiveWriteBuildWorkspace(t, workspace) })
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{"Aila", "Prompt"}, 20*time.Second)
	if _, err := terminal.Write([]byte("create a note file for this workspace\r")); err != nil {
		t.Fatalf("send denied interactive write prompt input: %v", err)
	}
	readUntilAll(t, terminal, []string{"Approval pending:", "proposal id: approval-call-write-1", "path: docs/interactive-build-output.md"}, 10*time.Second)
	if _, err := terminal.Write([]byte("n")); err != nil {
		t.Fatalf("send interactive write denial input: %v", err)
	}
	readUntilAll(t, terminal, []string{"Runtime idle", "approval deny: docs/interactive-build-output.md"}, 10*time.Second)
	if _, err := os.Stat(filepath.Join(workspace, "docs", "interactive-build-output.md")); !os.IsNotExist(err) {
		t.Fatalf("denied interactive write created file: %v", err)
	}
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after interactive write denial smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("interactive write denial PTY quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("interactive write denial PTY did not quit cleanly: %v", ctx.Err())
	}
}

func TestShellPrefixCommandPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	ctx, cancel, terminal, wait, _ := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 160, 45, env.vars, func(workspace string) {
		runPTYGit(t, workspace, "init")
		if err := os.WriteFile(filepath.Join(workspace, "smoke.txt"), []byte("shell prefix smoke\n"), 0o644); err != nil {
			t.Fatalf("write shell prefix smoke file: %v", err)
		}
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{"Aila", "Prompt"}, 20*time.Second)
	if _, err := terminal.Write([]byte("!git status --short\r")); err != nil {
		t.Fatalf("send shell prefix command: %v", err)
	}
	readUntilAll(t, terminal, []string{
		"Bash command:",
		"status: completed",
		"command: git status --short",
		"?? smoke.txt",
		"decision source: autonomy_policy",
	}, 10*time.Second)
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after shell prefix smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("shell prefix PTY quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("shell prefix PTY did not quit cleanly: %v", ctx.Err())
	}
}

func TestSummarizedShellContextPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	ctx, cancel, terminal, wait, _ := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 200, 60, env.vars, func(workspace string) {
		runPTYGit(t, workspace, "init")
		if err := os.WriteFile(filepath.Join(workspace, "context-smoke.txt"), []byte("summarized shell context\n"), 0o644); err != nil {
			t.Fatalf("write summarized shell context smoke file: %v", err)
		}
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{"Aila", "Prompt"}, 20*time.Second)
	if _, err := terminal.Write([]byte("!!git status --short\r")); err != nil {
		t.Fatalf("send summarized shell command: %v", err)
	}
	readUntilAll(t, terminal, []string{
		"Context:",
		"meter:",
		"claim: command git status --short completed exit 0",
		"source ref: command-1-stdout-1 command_stdout",
		"?? context-smoke.txt",
		"Bash command:",
		"command family: summarized shell",
	}, 10*time.Second)
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after summarized shell context smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("summarized shell context PTY quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("summarized shell context PTY did not quit cleanly: %v", ctx.Err())
	}
}

func TestManualCompactCommandPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	_, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 200, 100, env.vars, func(workspace string) {
		runPTYGit(t, workspace, "init")
		if err := os.WriteFile(filepath.Join(workspace, "compact-smoke.txt"), []byte("manual compact smoke\n"), 0o644); err != nil {
			t.Fatalf("write compact smoke file: %v", err)
		}
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{"Aila", "Prompt"}, 20*time.Second)
	if _, err := terminal.Write([]byte("!!git status --short\r")); err != nil {
		t.Fatalf("send summarized shell command before compact: %v", err)
	}
	readUntilAll(t, terminal, []string{
		"Context:",
		"claim: command git status --short completed exit 0",
		"source ref: command-1-stdout-1 command_stdout",
		"?? compact-smoke.txt",
	}, 10*time.Second)

	if _, err := terminal.Write([]byte("/compact\r")); err != nil {
		t.Fatalf("send manual compact command: %v", err)
	}
	output := readUntilAll(t, terminal, []string{
		"detail: manual context compaction",
		"Compact:",
		"status: completed",
		"summary: manual compaction preserved 5 source refs",
		"compact source ref: command-1-stdout-2 command_stdout",
		"Context:",
		"block: compacted_context Compacted context",
		"claim: manual compaction preserved 5 source refs",
		"?? compact-smoke.txt",
	}, 10*time.Second)
	for _, forbidden := range []string{workspace, env.home, env.xdgConfigHome, "app-owned manual compaction unavailable", "background compaction"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("manual compact PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

	drained := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, terminal)
		close(drained)
	}()
	time.Sleep(200 * time.Millisecond)
	if _, err := terminal.Write([]byte("/quit\r")); err != nil {
		t.Fatalf("send /quit after manual compact smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("manual compact PTY quit returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		cancel()
		_ = terminal.Close()
		t.Fatal("manual compact PTY did not quit after /quit")
	}
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("manual compact PTY drain did not finish after quit")
	}
}

func TestPromptPastePTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	ctx, cancel, terminal, wait, _ := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 120, 32, env.vars, func(workspace string) {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nPasted prompt smoke fixture.\n"), 0o644); err != nil {
			t.Fatalf("seed README for paste PTY smoke: %v", err)
		}
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{"Aila", "Prompt"}, 20*time.Second)
	if _, err := terminal.Write([]byte("summarize pasted prompt words\r")); err != nil {
		t.Fatalf("send pasted prompt input: %v", err)
	}
	readUntilAll(t, terminal, []string{
		"Runtime idle",
		"user: summarize pasted prompt words",
		"Read tool:",
		"path: README.md",
	}, 10*time.Second)
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after paste smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("paste PTY quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("paste PTY did not quit cleanly: %v", ctx.Err())
	}
}

func TestPromptInputUXFamilyPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	t.Run("editor", func(t *testing.T) {
		env := newAilaPTYEnv(t)
		editorDir := t.TempDir()
		editorPath := filepath.Join(editorDir, "fake-editor")
		editorScript := "#!/bin/sh\nprintf 'edited from fake editor\\nline two\\nline three' > \"$1\"\n"
		if err := os.WriteFile(editorPath, []byte(editorScript), 0o755); err != nil {
			t.Fatalf("write fake editor: %v", err)
		}
		env.vars = append(env.vars, "EDITOR="+editorPath)
		ctx, cancel, terminal, wait, _ := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 160, 45, env.vars, func(workspace string) {
			if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# README.md\n"), 0o644); err != nil {
				t.Fatalf("write editor smoke README: %v", err)
			}
		})
		defer cancel()
		defer func() { _ = terminal.Close() }()

		readUntilAll(t, terminal, []string{"Aila", "Prompt"}, 20*time.Second)
		if _, err := terminal.Write([]byte("/editor\r")); err != nil {
			t.Fatalf("send editor command: %v", err)
		}
		readUntilAll(t, terminal, []string{"editor:", "status: applied", "[Pasted lines +3]"}, 10*time.Second)
		if _, err := terminal.Write([]byte("\r")); err != nil {
			t.Fatalf("submit edited prompt: %v", err)
		}
		readUntilAll(t, terminal, []string{"Runtime idle", "user: edited from fake editor line two line three", "path: README.md"}, 10*time.Second)
		quitPromptInputUXPTY(t, terminal, wait, ctx, "editor")
	})

	t.Run("file reference", func(t *testing.T) {
		env := newAilaPTYEnv(t)
		ctx, cancel, terminal, wait, _ := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 160, 45, env.vars, func(workspace string) {
			for _, file := range []string{"README.md", "docs/guide.md"} {
				full := filepath.Join(workspace, filepath.FromSlash(file))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatalf("create prompt UX fixture dir: %v", err)
				}
				if err := os.WriteFile(full, []byte("# "+file+"\n"), 0o644); err != nil {
					t.Fatalf("write prompt UX fixture %s: %v", file, err)
				}
			}
		})
		defer cancel()
		defer func() { _ = terminal.Close() }()

		readUntilAll(t, terminal, []string{"Aila", "Prompt"}, 20*time.Second)
		if _, err := terminal.Write([]byte("@")); err != nil {
			t.Fatalf("open file-reference picker: %v", err)
		}
		readUntilAll(t, terminal, []string{"file-reference:", "source: app.file-reference", "README.md"}, 10*time.Second)
		if _, err := terminal.Write([]byte("\x1b[B\r")); err != nil {
			t.Fatalf("select file reference: %v", err)
		}
		readUntilAll(t, terminal, []string{"status: inserted", "> @docs/guide.md"}, 10*time.Second)
		if _, err := terminal.Write([]byte("\r")); err != nil {
			t.Fatalf("submit file-reference prompt: %v", err)
		}
		readUntilAll(t, terminal, []string{"Runtime idle", "user: @docs/guide.md", "path: README.md"}, 10*time.Second)
		quitPromptInputUXPTY(t, terminal, wait, ctx, "file reference")
	})

	t.Run("paste and resize", func(t *testing.T) {
		env := newAilaPTYEnv(t)
		ctx, cancel, terminal, wait, _ := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 160, 45, env.vars, func(workspace string) {
			if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# README.md\n"), 0o644); err != nil {
				t.Fatalf("write paste smoke README: %v", err)
			}
		})
		defer cancel()
		defer func() { _ = terminal.Close() }()

		readUntilAll(t, terminal, []string{"Aila", "Prompt"}, 20*time.Second)
		paste := "\x1b[200~alpha\nbeta\ngamma\n\x1b[201~"
		if _, err := terminal.Write([]byte(paste)); err != nil {
			t.Fatalf("send bracketed multiline paste: %v", err)
		}
		readUntilAll(t, terminal, []string{"[Pasted lines +4]"}, 10*time.Second)
		if _, err := terminal.Write([]byte("\r")); err != nil {
			t.Fatalf("submit pasted prompt: %v", err)
		}
		readUntilAll(t, terminal, []string{"Runtime idle", "user: alpha beta gamma", "path: README.md"}, 10*time.Second)
		if err := pty.Setsize(terminal, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
			t.Fatalf("resize prompt UX PTY: %v", err)
		}
		readUntilAll(t, terminal, []string{"80x24", "Prompt", "git: placeholder | context: placeholder | q quit"}, 10*time.Second)
		quitPromptInputUXPTY(t, terminal, wait, ctx, "paste and resize")
	})
}

func quitPromptInputUXPTY(t *testing.T, terminal *os.File, wait <-chan error, ctx context.Context, label string) {
	t.Helper()

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after %s prompt UX smoke: %v", label, err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("%s prompt input UX PTY quit returned error: %v", label, err)
		}
	case <-ctx.Done():
		t.Fatalf("%s prompt input UX PTY did not quit cleanly: %v", label, ctx.Err())
	}
}

func TestReadOnlyProviderFailurePTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	env.vars = append(env.vars, "AILA_AGENT_FAILURE=provider_auth_failed")
	ctx, cancel, terminal, wait := startAilaPTYWithSizeAndEnv(t, 160, 45, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntil(t, terminal, "Aila", 20*time.Second)
	if _, err := terminal.Write([]byte("trigger provider failure\r")); err != nil {
		t.Fatalf("send provider failure input: %v", err)
	}

	output := readUntilAll(t, terminal, []string{
		"provider_auth_failed: provider authentication failed",
		"source: provider",
		"affected artifact: provider_request",
		"assistant: provider authentication failed",
	}, 10*time.Second)
	if strings.Contains(output, "api_key=") || strings.Contains(output, "Authorization") {
		t.Fatalf("provider failure PTY leaked credential-shaped text: %q", output)
	}
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after provider failure: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("provider failure PTY quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("provider failure PTY did not quit cleanly: %v", ctx.Err())
	}
}

func TestSubmitWhileActivePTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	env.vars = append(env.vars, "AILA_FAKE_RUNTIME_HOLD_ACTIVE=1")
	ctx, cancel, terminal, wait := startAilaPTYWithSizeAndEnv(t, 160, 45, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntil(t, terminal, "Aila", 20*time.Second)
	if _, err := terminal.Write([]byte("slow active prompt\r")); err != nil {
		t.Fatalf("send active prompt input: %v", err)
	}

	active := readUntilAll(t, terminal, []string{
		"Runtime active",
		"Runtime status:",
		"status: active",
		"active: true",
		"user: slow active prompt",
	}, 10*time.Second)
	if strings.Contains(active, "active: false") {
		t.Fatalf("active window was hidden before queued submit: %q", active)
	}

	if _, err := terminal.Write([]byte("queued from active window\r")); err != nil {
		t.Fatalf("send queued prompt input: %v", err)
	}
	queued := readUntilAll(t, terminal, []string{
		"Queued input:",
		"queued messages: 1",
		"default action: send after current turn",
		"queued: queued from active window",
	}, 10*time.Second)
	if strings.Contains(queued, "active: false") {
		t.Fatalf("queued intent appeared only after active work was hidden: %q", queued)
	}
	if combined := active + queued; !strings.Contains(combined, "user: slow active prompt") || !strings.Contains(combined, "queued: queued from active window") {
		t.Fatalf("PTY output missing active context or queued intent: %q", combined)
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after queued smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("submit-while-active TUI quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("submit-while-active TUI did not clean up before timeout: %v", ctx.Err())
	}

	if _, err := os.Stat(filepath.Join(env.xdgConfigHome, "aila", "config.toml")); err != nil {
		t.Fatalf("temp XDG config was not created for scrubbed PTY startup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.home, ".config", "aila")); !os.IsNotExist(err) {
		t.Fatalf("scrubbed PTY touched HOME config path, err=%v", err)
	}
}

func TestInterruptActiveWorkPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	env.vars = append(env.vars,
		"AILA_FAKE_RUNTIME_HOLD_ACTIVE=1",
		"AILA_FAKE_RUNTIME_RESOLVE_SECOND_INTERRUPT=1",
	)
	ctx, cancel, terminal, wait := startAilaPTYWithSizeAndEnv(t, 160, 45, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntil(t, terminal, "Aila", 20*time.Second)
	if _, err := terminal.Write([]byte("interruptible fake work\r")); err != nil {
		t.Fatalf("send active prompt input: %v", err)
	}
	active := readUntilAll(t, terminal, []string{
		"Runtime active",
		"Runtime status:",
		"status: active",
		"active: true",
		"user: interruptible fake work",
	}, 10*time.Second)

	if _, err := terminal.Write([]byte{0x03}); err != nil {
		t.Fatalf("send ctrl-c interrupt input: %v", err)
	}
	canceling := readUntilAll(t, terminal, []string{
		"Runtime canceling",
		"status: canceling",
		"active: true",
		"interrupt state:",
		"interrupt status: canceling",
		"interrupt outcome: pending",
		"lower-layer cancellation executed: false",
		"user: interruptible fake work",
	}, 10*time.Second)

	if _, err := terminal.Write([]byte{0x03}); err != nil {
		t.Fatalf("send second ctrl-c fake interrupt resolution input: %v", err)
	}
	canceled := readUntilAll(t, terminal, []string{
		"Runtime canceled",
		"status: canceled",
		"active: false",
		"result: fake work canceled",
		"interrupt state:",
		"interrupt status: canceled",
		"interrupt outcome: fake work canceled",
		"lower-layer cancellation executed: false",
		"user: interruptible fake work",
	}, 10*time.Second)

	combined := active + canceling + canceled
	for _, forbidden := range []string{"real IO cancellation", "tool cancellation", "provider cancellation", "shell cancellation", "lower-layer cancellation executed: true"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("interrupt PTY output claimed lower-layer cancellation marker %q: %q", forbidden, combined)
		}
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after interrupt smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("interrupt TUI quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("interrupt TUI did not clean up before timeout: %v", ctx.Err())
	}

	if _, err := os.Stat(filepath.Join(env.xdgConfigHome, "aila", "config.toml")); err != nil {
		t.Fatalf("temp XDG config was not created for scrubbed PTY startup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.home, ".config", "aila")); !os.IsNotExist(err) {
		t.Fatalf("scrubbed PTY touched HOME config path, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(env.home, ".aila")); !os.IsNotExist(err) {
		t.Fatalf("interrupt PTY touched HOME .aila state, err=%v", err)
	}
}

func TestReviewCommandShowsAuditFindingsPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	_, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 200, 100, env.vars, func(workspace string) {
		seedDiffSmokeWorkspace(t, workspace)
		seedFakeHistoryEvents(t, workspace)
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)
	trackedStatusBefore := runPTYGitOutput(t, workspace, "status", "--short", "--", "internal/demo.txt")

	if _, err := terminal.Write([]byte("/review\r")); err != nil {
		t.Fatalf("send /review command input: %v", err)
	}
	output := readUntilAll(t, terminal, []string{
		"Runtime status:",
		"detail: audit capability status",
		"result: Audit found 1 changed file(s) needing review.",
		"Audit:",
		"source: app.audit",
		"capability: audit",
		"signal: flagged",
		"evidence: diff_available",
		"recommended successor: build",
		"successor valid: true",
		"successor rejected: false",
		"transition claimed: false",
		"display-only: true",
		"summary: Audit found 1 changed file(s) needing review.",
		"finding: current-change-review severity=warning title=Review current changes before continuing",
		"finding message: current-change-review 1 changed file(s) need review before another build step.",
		"finding source refs: current-change-review review-diff",
		"finding next action: current-change-review Route back to build after reviewing changed files.",
		"requested boundary: artifact_access operation=artifact.access target=history",
		"source ref: review-diff kind=diff path=internal/demo.txt excerpt=status=ready changed_files=1",
		"source ref: review-history kind=history excerpt=state=loaded events=6",
	}, 10*time.Second)
	assertNoDiffSmokeLeaks(t, output, env, workspace)
	for _, forbidden := range []string{"provider-backed", "provider review", "transition claimed: true", "Approval pending:", "path: docs/aila-build-output.md"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("review audit PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

	assertFileContent(t, filepath.Join(workspace, "internal", "demo.txt"), "new value\nsecond value\n")
	trackedStatusAfter := runPTYGitOutput(t, workspace, "status", "--short", "--", "internal/demo.txt")
	if trackedStatusAfter != trackedStatusBefore {
		t.Fatalf("review audit PTY changed tracked git status: before=%q after=%q", trackedStatusBefore, trackedStatusAfter)
	}
	if docsStatus := runPTYGitOutput(t, workspace, "status", "--short", "--", "docs/aila-build-output.md"); docsStatus != "" {
		t.Fatalf("review audit PTY changed build output git status: %q", docsStatus)
	}
	if _, err := os.Stat(filepath.Join(workspace, "docs", "aila-build-output.md")); !os.IsNotExist(err) {
		t.Fatalf("review audit PTY unexpectedly created build output, err=%v", err)
	}

	cancel()
	_ = terminal.Close()
	select {
	case <-wait:
	case <-time.After(5 * time.Second):
		t.Fatal("review audit PTY did not stop after cancellation")
	}

	assertNoDurableStatePollution(t, env, baseline)
}

func TestInspectionCommandFamilyPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 160, 60, env.vars, func(workspace string) {
		seedDiffSmokeWorkspace(t, workspace)
		seedFakeHistoryEvents(t, workspace)
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)

	if _, err := terminal.Write([]byte{0x18, 's'}); err != nil {
		t.Fatalf("send ctrl+x s command input: %v", err)
	}
	status := readUntilAll(t, terminal, []string{
		"Runtime status:",
		"detail: brief capability status",
		"result: Brief: phase idle",
		"Brief:",
		"capability: brief",
		"current phase: idle",
		"runtime status: idle",
		"display-only: true",
		"suggested next action:",
		"requested boundary: state_access",
		"source ref: brief-runtime",
		"transition claimed: false",
	}, 10*time.Second)
	assertNoDiffSmokeLeaks(t, status, env, workspace)
	for _, forbidden := range []string{"Deterministic placeholder status", "real status sources: deferred", "provider review"} {
		if strings.Contains(status, forbidden) {
			t.Fatalf("status inspection PTY output exposed forbidden marker %q: %q", forbidden, status)
		}
	}

	if _, err := terminal.Write([]byte("/plan\r")); err != nil {
		t.Fatalf("send /plan command input: %v", err)
	}
	plan := readUntilAll(t, terminal, []string{
		"Plan:",
		"capability: plan",
		"signal: complete",
		"artifact status: written",
		"item: scope status=done",
		"item: implement status=pending",
		"next action: Review the plan artifact",
		"successor valid: true",
		"transition claimed: false",
		"requested boundary: state_write",
		"source ref: plan-project-state",
	}, 10*time.Second)
	assertNoDiffSmokeLeaks(t, plan, env, workspace)
	planArtifact, err := os.ReadFile(filepath.Join(workspace, ".aila", "artifacts", "plan.md"))
	if err != nil {
		t.Fatalf("read plan artifact: %v", err)
	}
	if !strings.Contains(string(planArtifact), "# Current Session Plan") || !strings.Contains(string(planArtifact), "GIVEN implementation starts WHEN code changes are made") {
		t.Fatalf("plan artifact content missing expected scope or acceptance criteria:\n%s", planArtifact)
	}

	if _, err := terminal.Write([]byte("/review\r")); err != nil {
		t.Fatalf("send /review command input: %v", err)
	}
	review := readUntilAll(t, terminal, []string{
		"Audit:",
		"source: app.audit",
		"capability: audit",
		"signal: flagged",
		"evidence: diff_available",
		"summary: Audit found 1 changed file(s) needing review.",
		"finding: current-change-review severity=warning title=Review current changes before continuing",
		"finding source refs: current-change-review review-diff",
		"successor valid: true",
		"transition claimed: false",
		"requested boundary: artifact_access operation=artifact.access target=history",
		"source ref: review-diff kind=diff path=internal/demo.txt excerpt=status=ready changed_files=1",
		"source ref: review-history kind=history excerpt=state=loaded events=10",
	}, 10*time.Second)
	assertNoDiffSmokeLeaks(t, review, env, workspace)
	for _, forbidden := range []string{"provider-backed", "provider review", "model switch", "autonomy switch"} {
		if strings.Contains(review, forbidden) {
			t.Fatalf("review inspection PTY output exposed forbidden marker %q: %q", forbidden, review)
		}
	}

	if _, err := terminal.Write([]byte("/history\r")); err != nil {
		t.Fatalf("send /history command input: %v", err)
	}
	historyOutput := readUntilAll(t, terminal, []string{
		"history:",
		"read-only: true",
		"history-run history-session history-event-1 prompt user asked for fake history",
		"history-run history-session history-event-5 mutation write completed notes.txt",
		"selected event id: history-event-1",
		"selected run id: history-run",
		"selected session id: history-session",
	}, 10*time.Second)
	assertNoHistorySmokeLeaks(t, historyOutput, env, workspace)
	if _, err := terminal.Write([]byte{0x18, 'd'}); err != nil {
		t.Fatalf("send ctrl+x d command input: %v", err)
	}
	diffOutput := readUntilAll(t, terminal, []string{
		"diff:",
		"source: git diff",
		"status: ready",
		"file: internal/demo.txt status: modified",
		"- old value",
		"+ new value",
	}, 10*time.Second)
	assertNoDiffSmokeLeaks(t, diffOutput, env, workspace)
	if _, err := terminal.Write([]byte{0x1b}); err != nil {
		t.Fatalf("send Escape after diff inspection: %v", err)
	}
	readUntilAll(t, terminal, []string{"Display labels:", "project store: initialized - project store ready", "Runtime status:"}, 10*time.Second)

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after inspection smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("inspection PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("inspection PTY did not clean up before timeout: %v", ctx.Err())
	}

	assertInspectionCommandFamilyStoreState(t, workspace)
	assertNoDurableStatePollution(t, env, baseline)
}

func TestVisionCommandPersistsGoalArtifactPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 200, 100, env.vars, func(workspace string) {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nVision command PTY fixture.\n"), 0o644); err != nil {
			t.Fatalf("seed vision PTY README: %v", err)
		}
		runPTYGit(t, workspace, "init")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "add", "README.md")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)
	trackedStatusBefore := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")

	if _, err := terminal.Write([]byte("/vision\r")); err != nil {
		t.Fatalf("send /vision command input: %v", err)
	}
	output := readUntilAll(t, terminal, []string{
		"Runtime status:",
		"result: Vision shaped project direction and long-term goals.",
		"Vision:",
		"source: app.vision",
		"capability: vision",
		"signal: complete",
		"phase: envision",
		"artifact status: written",
		"recommended successor: plan",
		"successor valid: true",
		"transition claimed: false",
		"display-only: true",
		"north star: Shape Aila's project direction before planning broad work.",
		"principle: Keep Aila a fixed terminal coding agent rather than a plugin host.",
		"long-term goal: Use persisted vision as source material for later plan and build work.",
		"requested boundary: state_write operation=state.write target=vision",
		"source ref: vision-command kind=command command=/vision",
	}, 10*time.Second)
	for _, forbidden := range []string{"Approval pending:", "Build:", "Audit:", "Discuss:", "provider-backed strategy", "transition claimed: true"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("vision PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

	artifact, err := os.ReadFile(filepath.Join(workspace, ".aila", "artifacts", "vision.md"))
	if err != nil {
		t.Fatalf("read vision artifact: %v", err)
	}
	for _, want := range []string{"# Vision", "North star: Shape Aila's project direction before planning broad work.", "## Principles", "Next action: Use this vision as source material for planning."} {
		if !strings.Contains(string(artifact), want) {
			t.Fatalf("vision artifact missing %q in:\n%s", want, artifact)
		}
	}
	trackedStatusAfter := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")
	if trackedStatusAfter != trackedStatusBefore {
		t.Fatalf("vision PTY changed tracked git status: before=%q after=%q", trackedStatusBefore, trackedStatusAfter)
	}
	if docsStatus := runPTYGitOutput(t, workspace, "status", "--short", "--", "docs/aila-build-output.md"); docsStatus != "" {
		t.Fatalf("vision PTY changed build output git status: %q", docsStatus)
	}
	assertVisionCommandStoreState(t, workspace)

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after vision command smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("vision command PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("vision command PTY did not clean up before timeout: %v", ctx.Err())
	}

	assertNoDurableStatePollution(t, env, baseline)
}

func TestDiscussCommandPersistsDecisionArtifactPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 200, 100, env.vars, func(workspace string) {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nDiscuss command PTY fixture.\n"), 0o644); err != nil {
			t.Fatalf("seed discuss PTY README: %v", err)
		}
		runPTYGit(t, workspace, "init")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "add", "README.md")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)
	trackedStatusBefore := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")

	if _, err := terminal.Write([]byte("/discuss\r")); err != nil {
		t.Fatalf("send /discuss command input: %v", err)
	}
	output := readUntilAll(t, terminal, []string{
		"Runtime status:",
		"result: Discuss recorded a consequential decision.",
		"Discuss:",
		"source: app.discuss",
		"capability: discuss",
		"signal: complete",
		"phase: deliberate",
		"artifact status: written",
		"recommended successor: plan",
		"successor valid: true",
		"transition claimed: false",
		"display-only: true",
		"question: Decide the next safe workflow direction for Aila.",
		"option: option-1 selected=true text=Plan the scoped next step",
		"selected decision: Plan the scoped next step",
		"reasoning: Planning keeps the next step bounded and preserves workflow authority before build work.",
		"confidence: medium",
		"requested boundary: state_write operation=state.write target=decisions",
		"source ref: discuss-command kind=command command=/discuss",
	}, 10*time.Second)
	for _, forbidden := range []string{"Approval pending:", "Build:", "Audit:", "Vision:", "provider-backed strategy", "transition claimed: true"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("discuss PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

	artifact, err := os.ReadFile(filepath.Join(workspace, ".aila", "artifacts", "decisions.md"))
	if err != nil {
		t.Fatalf("read decision artifact: %v", err)
	}
	for _, want := range []string{"# Decision", "Question: Decide the next safe workflow direction for Aila.", "## Options", "Choice: Plan the scoped next step", "Next action: Use this decision as source material for planning."} {
		if !strings.Contains(string(artifact), want) {
			t.Fatalf("decision artifact missing %q in:\n%s", want, artifact)
		}
	}
	trackedStatusAfter := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")
	if trackedStatusAfter != trackedStatusBefore {
		t.Fatalf("discuss PTY changed tracked git status: before=%q after=%q", trackedStatusBefore, trackedStatusAfter)
	}
	if docsStatus := runPTYGitOutput(t, workspace, "status", "--short", "--", "docs/aila-build-output.md"); docsStatus != "" {
		t.Fatalf("discuss PTY changed build output git status: %q", docsStatus)
	}
	assertDiscussCommandStoreState(t, workspace)

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after discuss command smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("discuss command PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("discuss command PTY did not clean up before timeout: %v", ctx.Err())
	}

	assertNoDurableStatePollution(t, env, baseline)
}

func TestResearchCommandFoldsContextWithoutArtifactPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 200, 100, env.vars, func(workspace string) {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nResearch command PTY fixture.\n"), 0o644); err != nil {
			t.Fatalf("seed research PTY README: %v", err)
		}
		runPTYGit(t, workspace, "init")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "add", "README.md")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)
	trackedStatusBefore := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")

	if _, err := terminal.Write([]byte("/research\r")); err != nil {
		t.Fatalf("send /research command input: %v", err)
	}
	output := readUntilAll(t, terminal, []string{
		"Runtime status:",
		"result: Research folded",
		"Research:",
		"source: app.research",
		"capability: research",
		"signal: complete",
		"current phase: idle",
		"cross-cutting status: context_only",
		"context folded: true",
		"recommended successor:",
		"transition claimed: false",
		"display-only: true",
		"topic: Research external patterns for Aila.",
		"pattern: pattern-1 concept=Cross-cutting helpers return context evidence without owning phase transitions",
		"evidence: evidence-1 summary=docs/workflow-architecture.md keeps research cross-cutting",
		"confidence: medium",
		"caveat: deterministic app-supplied pattern evidence only",
		"Context:",
		"source: app.research.context",
		"status: folded",
		"meter: research refs: 4",
		"claim: research pattern: Cross-cutting helpers return context evidence without owning phase transitions",
		"source ref: research-command kind=command command=/research",
	}, 10*time.Second)
	for _, forbidden := range []string{"Approval pending:", "Build:", "Audit:", "Vision:", "Discuss:", "state_write", "transition claimed: true"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("research PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

	if _, err := os.Stat(filepath.Join(workspace, ".aila", "artifacts", "research.md")); !os.IsNotExist(err) {
		t.Fatalf("research command should not write research artifact, stat err=%v", err)
	}
	trackedStatusAfter := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")
	if trackedStatusAfter != trackedStatusBefore {
		t.Fatalf("research PTY changed tracked git status: before=%q after=%q", trackedStatusBefore, trackedStatusAfter)
	}
	if docsStatus := runPTYGitOutput(t, workspace, "status", "--short", "--", "docs/aila-build-output.md"); docsStatus != "" {
		t.Fatalf("research PTY changed build output git status: %q", docsStatus)
	}
	assertResearchCommandStoreState(t, workspace)

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after research command smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("research command PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("research command PTY did not clean up before timeout: %v", ctx.Err())
	}

	assertNoDurableStatePollution(t, env, baseline)
}

func TestProfileCommandPersistsArtifactAndFoldsContextPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 200, 100, env.vars, func(workspace string) {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nProfile command PTY fixture.\n"), 0o644); err != nil {
			t.Fatalf("seed profile PTY README: %v", err)
		}
		runPTYGit(t, workspace, "init")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "add", "README.md")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)
	trackedStatusBefore := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")

	if _, err := terminal.Write([]byte("/profile\r")); err != nil {
		t.Fatalf("send /profile command input: %v", err)
	}
	output := readUntilAll(t, terminal, []string{
		"Runtime status:",
		"result: Profile folded",
		"Profile:",
		"source: app.profile",
		"capability: profile",
		"signal: complete",
		"current phase: idle",
		"cross-cutting status: context_only",
		"context folded: true",
		"artifact status: written",
		"recommended successor:",
		"transition claimed: false",
		"display-only: true",
		"subject: Aila decision profile",
		"decision signal: signal-1 pattern=Prefer bounded roadmap slices before broad refactors",
		"update suggestion: suggestion-1 text=Keep capability validation evidence near the closeout artifact",
		"evidence: evidence-1 summary=Recent roadmap work used planera before implementation",
		"confidence: medium",
		"caveat: deterministic app-supplied session evidence only",
		"requested boundary: state_write operation=state.write target=profile",
		"source ref: profile-command kind=command command=/profile",
		"Context:",
		"source: app.profile.context",
		"status: folded",
		"meter: profile refs: 4",
		"claim: profile signal: Prefer bounded roadmap slices before broad refactors",
		"source ref: profile-command command command=/profile",
	}, 10*time.Second)
	for _, forbidden := range []string{"Approval pending:", "Build:", "Audit:", "Vision:", "Discuss:", "Research:", "transition claimed: true"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("profile PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

	artifact, err := os.ReadFile(filepath.Join(workspace, ".aila", "artifacts", "profile.md"))
	if err != nil {
		t.Fatalf("read profile artifact: %v", err)
	}
	for _, want := range []string{"# Profile", "Subject: Aila decision profile", "## Decision Signals", "Prefer bounded roadmap slices before broad refactors", "## Update Suggestions", "Keep capability validation evidence near the closeout artifact", "## Evidence", "Recent roadmap work used planera before implementation", "## Caveats", "Next action: Use this profile as non-authoritative context for the current workflow phase."} {
		if !strings.Contains(string(artifact), want) {
			t.Fatalf("profile artifact missing %q in:\n%s", want, artifact)
		}
	}
	trackedStatusAfter := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")
	if trackedStatusAfter != trackedStatusBefore {
		t.Fatalf("profile PTY changed tracked git status: before=%q after=%q", trackedStatusBefore, trackedStatusAfter)
	}
	if docsStatus := runPTYGitOutput(t, workspace, "status", "--short", "--", "docs/aila-build-output.md"); docsStatus != "" {
		t.Fatalf("profile PTY changed build output git status: %q", docsStatus)
	}
	assertProfileCommandStoreState(t, workspace)

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after profile command smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("profile command PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("profile command PTY did not clean up before timeout: %v", ctx.Err())
	}

	assertNoDurableStatePollution(t, env, baseline)
}

func TestOptimizeCommandPersistsMetricArtifactsPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	_, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 200, 100, env.vars, func(workspace string) {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nOptimize command PTY fixture.\n"), 0o644); err != nil {
			t.Fatalf("seed optimize PTY README: %v", err)
		}
		runPTYGit(t, workspace, "init")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "add", "README.md")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)
	trackedStatusBefore := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")

	if _, err := terminal.Write([]byte("/optimize\r")); err != nil {
		t.Fatalf("send /optimize command input: %v", err)
	}
	output := readUntilAll(t, terminal, []string{
		"Runtime status:",
		"result: Optimize measured render_evidence_seconds",
		"Optimize:",
		"source: app.optimize",
		"capability: optimize",
		"signal: complete",
		"phase: build",
		"objective: current-metric-objective",
		"experiment: experiment-current-render-evidence status=improved",
		"harness: fixture-metric-harness locked=true",
		"metric: render_evidence_seconds baseline=1.50s result=1.20s",
		"transition claimed: false",
		"display-only: true",
		"recommended successor: audit",
		"metric improvement: 20.0% lower",
		"next action: Audit the measured optimization result before continuing.",
		"evidence: evidence-1 summary=objective selected from current BUILD context",
		"caveat: deterministic app-supplied metric evidence only",
		"artifact status: written",
		"requested boundary: tool_execution operation=bash",
		"requested boundary: permission_check operation=bash",
		"source ref: optimize-command kind=command command=/optimize",
	}, 10*time.Second)
	for _, forbidden := range []string{"Approval pending:", "Build:", "Audit:", "Vision:", "Discuss:", "Research:", "Profile:", "Document:", "Design:", "transition claimed: true"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("optimize PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

	objective, err := os.ReadFile(filepath.Join(workspace, ".aila", "artifacts", "objective.md"))
	if err != nil {
		t.Fatalf("read optimize objective artifact: %v", err)
	}
	for _, want := range []string{"# Optimization Objective", "ID: current-metric-objective", "Reduce evidence rendering latency without changing workflow authority."} {
		if !strings.Contains(string(objective), want) {
			t.Fatalf("objective artifact missing %q in:\n%s", want, objective)
		}
	}
	experiment, err := os.ReadFile(filepath.Join(workspace, ".aila", "artifacts", "experiments.md"))
	if err != nil {
		t.Fatalf("read optimize experiments artifact: %v", err)
	}
	for _, want := range []string{"# Optimization Experiment", "Experiment: experiment-current-render-evidence", "Status: improved", "Harness: locked TUI fixture metric comparison locked=true", "Metric: render_evidence_seconds 1.50s -> 1.20s (20.0% lower)"} {
		if !strings.Contains(string(experiment), want) {
			t.Fatalf("experiments artifact missing %q in:\n%s", want, experiment)
		}
	}
	trackedStatusAfter := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")
	if trackedStatusAfter != trackedStatusBefore {
		t.Fatalf("optimize PTY changed tracked git status: before=%q after=%q", trackedStatusBefore, trackedStatusAfter)
	}
	if docsStatus := runPTYGitOutput(t, workspace, "status", "--short", "--", "docs/aila-build-output.md"); docsStatus != "" {
		t.Fatalf("optimize PTY changed build output git status: %q", docsStatus)
	}
	assertOptimizeCommandStoreState(t, workspace)

	drained := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, terminal)
		close(drained)
	}()
	time.Sleep(200 * time.Millisecond)
	if _, err := terminal.Write([]byte("/quit\r")); err != nil {
		t.Fatalf("send /quit input after optimize command smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("optimize command PTY returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		cancel()
		_ = terminal.Close()
		t.Fatal("optimize command PTY did not quit after /quit")
	}
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("optimize command PTY drain did not finish after quit")
	}

	assertNoDurableStatePollution(t, env, baseline)
}

func TestDocumentCommandWritesDocsThroughMutationPathPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	_, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 200, 100, env.vars, func(workspace string) {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nDocument command PTY fixture.\n"), 0o644); err != nil {
			t.Fatalf("seed document PTY README: %v", err)
		}
		runPTYGit(t, workspace, "init")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "add", "README.md")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)
	trackedStatusBefore := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")

	if _, err := terminal.Write([]byte("/document\r")); err != nil {
		t.Fatalf("send /document command input: %v", err)
	}
	output := readUntilAll(t, terminal, []string{
		"Runtime status:",
		"result: Document aligned docs/aila-documentation-output.md",
		"Document:",
		"source: app.document",
		"capability: document",
		"signal: complete",
		"phase: build",
		"target: docs/aila-documentation-output.md",
		"plan: document-command-safety",
		"mutation: write status=completed path=docs/aila-documentation-output.md",
		"transition claimed: false",
		"display-only: true",
		"recommended successor: audit",
		"output: Documented the /document command mutation safety path.",
		"changed doc: docs/aila-documentation-output.md status=completed",
		"doc diff: + # Aila Documentation Alignment",
		"caveat: deterministic app-supplied documentation evidence only",
		"artifact status: written",
		"requested boundary: tool_execution operation=write target=docs/aila-documentation-output.md",
		"requested boundary: permission_check operation=write target=docs/aila-documentation-output.md",
		"source ref: document-command kind=command command=/document",
	}, 10*time.Second)
	for _, forbidden := range []string{"Approval pending:", "Build:", "Audit:", "Vision:", "Discuss:", "Research:", "Profile:", "Optimize:", "Design:", "transition claimed: true"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("document PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

	document, err := os.ReadFile(filepath.Join(workspace, "docs", "aila-documentation-output.md"))
	if err != nil {
		t.Fatalf("read document output: %v", err)
	}
	for _, want := range []string{"# Aila Documentation Alignment", "Capability: document", "Target behavior: /document routes documentation writes through mutation safety."} {
		if !strings.Contains(string(document), want) {
			t.Fatalf("document output missing %q in:\n%s", want, document)
		}
	}
	artifact, err := os.ReadFile(filepath.Join(workspace, ".aila", "artifacts", "documentation.md"))
	if err != nil {
		t.Fatalf("read documentation artifact: %v", err)
	}
	for _, want := range []string{"# Documentation Alignment", "Target: docs/aila-documentation-output.md", "Documented the /document command mutation safety path."} {
		if !strings.Contains(string(artifact), want) {
			t.Fatalf("documentation artifact missing %q in:\n%s", want, artifact)
		}
	}
	trackedStatusAfter := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")
	if trackedStatusAfter != trackedStatusBefore {
		t.Fatalf("document PTY changed tracked git status: before=%q after=%q", trackedStatusBefore, trackedStatusAfter)
	}
	if docsStatus := runPTYGitOutput(t, workspace, "status", "--short", "--", "docs/aila-build-output.md"); docsStatus != "" {
		t.Fatalf("document PTY changed build output git status: %q", docsStatus)
	}
	assertDocumentCommandStoreState(t, workspace)

	drained := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, terminal)
		close(drained)
	}()
	time.Sleep(200 * time.Millisecond)
	if _, err := terminal.Write([]byte("/quit\r")); err != nil {
		t.Fatalf("send /quit input after document command smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("document command PTY returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatalf("document command PTY did not exit after /quit")
	}
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
	}

	assertNoDurableStatePollution(t, env, baseline)
}

func TestOrchestrateCommandShowsProgressSummaryPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	_, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 220, 110, env.vars, func(workspace string) {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nOrchestrate command PTY fixture.\n"), 0o644); err != nil {
			t.Fatalf("seed orchestrate PTY README: %v", err)
		}
		runPTYGit(t, workspace, "init")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "add", "README.md")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{"Aila", "Prompt"}, 20*time.Second)
	trackedStatusBefore := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")

	if _, err := terminal.Write([]byte("/orchestrate\r")); err != nil {
		t.Fatalf("send /orchestrate command input: %v", err)
	}
	output := readUntilAll(t, terminal, []string{
		"Runtime status:",
		"result: Orchestration completed 2 cycles for current-plan with 1 retry used.",
		"Orchestration:",
		"capability: orchestrate",
		"status: completed",
		"active cycle: cycle-2",
		"retry budget: max=1 used=1 remaining=0",
		"cycle: cycle-1 capability=build status=retrying",
		"child work: orchestrate-build-1 capability=build status=failed",
		"decision: retry-build kind=retry",
		"final summary: Coordinated two bounded cycles, retried one failed child, evaluated recovery, and stopped.",
		"transition claimed: false",
		"display-only: true",
	}, 20*time.Second)
	for _, forbidden := range []string{"plugin host", "MCP server", "graph engine", "marketplace", "lower-layer cancellation executed: true"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("orchestrate PTY output included forbidden boundary marker %q: %q", forbidden, output)
		}
	}
	if trackedStatusAfter := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md"); trackedStatusAfter != trackedStatusBefore {
		t.Fatalf("orchestrate PTY changed tracked git status: before=%q after=%q", trackedStatusBefore, trackedStatusAfter)
	}

	drained := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, terminal)
		close(drained)
	}()
	time.Sleep(200 * time.Millisecond)
	if _, err := terminal.Write([]byte("/quit\r")); err != nil {
		t.Fatalf("send /quit input after orchestrate command smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("orchestrate command PTY returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatalf("orchestrate command PTY did not exit after /quit")
	}
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
	}
	assertNoDurableStatePollution(t, env, baseline)
}

func TestOrchestrateCommandCancellationPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	env.vars = append(env.vars,
		"AILA_FAKE_RUNTIME_HOLD_ACTIVE=1",
		"AILA_FAKE_RUNTIME_RESOLVE_SECOND_INTERRUPT=1",
		"AILA_FAKE_ORCHESTRATE_HOLD_ACTIVE=1",
	)
	ctx, cancel, terminal, wait := startAilaPTYWithSizeAndEnv(t, 180, 70, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntil(t, terminal, "Aila", 20*time.Second)
	if _, err := terminal.Write([]byte("/orchestrate\r")); err != nil {
		t.Fatalf("send /orchestrate active input: %v", err)
	}
	active := readUntilAll(t, terminal, []string{
		"Runtime active",
		"status: active",
		"active: true",
		"Orchestration:",
		"status: running",
		"active cycle: cycle-1",
	}, 10*time.Second)

	if _, err := terminal.Write([]byte{0x03}); err != nil {
		t.Fatalf("send ctrl-c orchestrate interrupt input: %v", err)
	}
	canceling := readUntilAll(t, terminal, []string{
		"Runtime canceling",
		"status: canceling",
		"interrupt state:",
		"interrupt status: canceling",
		"lower-layer cancellation executed: false",
		"Orchestration:",
	}, 10*time.Second)

	if _, err := terminal.Write([]byte{0x03}); err != nil {
		t.Fatalf("send second ctrl-c orchestrate interrupt input: %v", err)
	}
	canceled := readUntilAll(t, terminal, []string{
		"Runtime canceled",
		"status: canceled",
		"active: false",
		"result: fake work canceled",
		"interrupt state:",
		"interrupt status: canceled",
		"lower-layer cancellation executed: false",
	}, 10*time.Second)
	combined := active + canceling + canceled
	for _, forbidden := range []string{"real IO cancellation", "tool cancellation", "provider cancellation", "shell cancellation", "lower-layer cancellation executed: true"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("orchestrate cancellation PTY output claimed lower-layer cancellation marker %q: %q", forbidden, combined)
		}
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after orchestrate interrupt smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("orchestrate interrupt PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("orchestrate interrupt PTY did not quit cleanly: %v", ctx.Err())
	}
}

func TestDesignCommandPersistsDesignArtifactPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	_, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 200, 100, env.vars, func(workspace string) {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nDesign command PTY fixture.\n"), 0o644); err != nil {
			t.Fatalf("seed design PTY README: %v", err)
		}
		runPTYGit(t, workspace, "init")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "add", "README.md")
		runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)
	trackedStatusBefore := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")

	if _, err := terminal.Write([]byte("/design\r")); err != nil {
		t.Fatalf("send /design command input: %v", err)
	}
	output := readUntilAll(t, terminal, []string{
		"Runtime status:",
		"result: Design recorded 3 decisions for aila-terminal-design-system.",
		"Design:",
		"source: app.design",
		"capability: design",
		"signal: complete",
		"phase: build",
		"goal: aila-terminal-design-system surface=terminal-ui",
		"artifact status: written",
		"visual review required: false",
		"transition claimed: false",
		"display-only: true",
		"recommended successor: audit",
		"decision: phase-hierarchy area=information architecture",
		"review prompt: desktop-hierarchy",
		"caveat: screenshots are review aids, not correctness contracts",
		"requested boundary: state_write operation=state.write target=design",
		"source ref: design-command kind=command command=/design",
	}, 10*time.Second)
	for _, forbidden := range []string{"Approval pending:", "Build:", "Audit:", "Vision:", "Discuss:", "Research:", "Profile:", "Optimize:", "Document:", "transition claimed: true", "screenshot correctness contract: true"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("design PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

	artifact, err := os.ReadFile(filepath.Join(workspace, ".aila", "artifacts", "design.md"))
	if err != nil {
		t.Fatalf("read design artifact: %v", err)
	}
	for _, want := range []string{"# Design System", "phase-hierarchy", "Visual Review Prompts", "desktop-hierarchy", "screenshots are review aids, not correctness contracts"} {
		if !strings.Contains(string(artifact), want) {
			t.Fatalf("design artifact missing %q in:\n%s", want, artifact)
		}
	}
	trackedStatusAfter := runPTYGitOutput(t, workspace, "status", "--short", "--", "README.md")
	if trackedStatusAfter != trackedStatusBefore {
		t.Fatalf("design PTY changed tracked git status: before=%q after=%q", trackedStatusBefore, trackedStatusAfter)
	}
	if docsStatus := runPTYGitOutput(t, workspace, "status", "--short", "--", "docs/aila-build-output.md"); docsStatus != "" {
		t.Fatalf("design PTY changed build output git status: %q", docsStatus)
	}
	assertDesignCommandStoreState(t, workspace)

	drained := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, terminal)
		close(drained)
	}()
	time.Sleep(200 * time.Millisecond)
	if _, err := terminal.Write([]byte("/quit\r")); err != nil {
		t.Fatalf("send /quit input after design command smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("design command PTY returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatalf("design command PTY did not exit after /quit")
	}
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
	}

	assertNoDurableStatePollution(t, env, baseline)
}

func TestBuildCommandExecutesOnePlannedStepPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	_, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 200, 70, env.vars, nil)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)

	if _, err := terminal.Write([]byte("/plan\r")); err != nil {
		t.Fatalf("send /plan command input before build: %v", err)
	}
	readUntilAll(t, terminal, []string{
		"Plan:",
		"item: scope status=done",
		"item: implement status=pending",
		"successor valid: true",
		"transition claimed: false",
	}, 10*time.Second)

	if _, err := terminal.Write([]byte("/build\r")); err != nil {
		t.Fatalf("send /build command input: %v", err)
	}
	output := readUntilAll(t, terminal, []string{
		"active: false",
		"Build:",
		"capability: build",
		"signal: complete",
		"plan item: implement status=active",
		"step: write-build-output status=completed",
		"tool: write status=completed",
		"path: docs/aila-build-output.md",
		"decision source: autonomy_policy",
		"decision autonomy: yolo",
		"approval required: false",
		"changed path: docs/aila-build-output.md",
		"final summary: Executed one bounded write step",
		"recommended successor: audit",
		"transition claimed: false",
	}, 10*time.Second)
	if strings.Contains(output, "Approval pending:") {
		t.Fatalf("build command PTY output exposed out-of-scope approval prompt: %q", output)
	}

	content, err := os.ReadFile(filepath.Join(workspace, "docs", "aila-build-output.md"))
	if err != nil {
		t.Fatalf("read build command output file: %v", err)
	}
	if !strings.Contains(string(content), "Plan item: Implement only the scoped plan behavior") || !strings.Contains(string(content), "executed one bounded build step and held") {
		t.Fatalf("build command output content = %q", content)
	}
	assertBuildCommandStoreState(t, workspace)

	drained := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, terminal)
		close(drained)
	}()
	time.Sleep(200 * time.Millisecond)
	if _, err := terminal.Write([]byte("/quit\r")); err != nil {
		t.Fatalf("send /quit input after build command smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("build command PTY returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		cancel()
		_ = terminal.Close()
		t.Fatal("build command PTY did not quit after /quit")
	}
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("build command PTY drain did not finish after quit")
	}

	assertNoDurableStatePollution(t, env, baseline)
}

func TestModelAndAutonomyCommandFamilyPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 160, 60, env.vars, nil)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"Model opencode-go/deepseek-v4-pro:high",
		"Utility opencode-go/deepseek-v4-flash:max",
		"Auto yolo",
		"Prompt",
	}, 20*time.Second)

	if _, err := terminal.Write([]byte("/model\r")); err != nil {
		t.Fatalf("send /model command input: %v", err)
	}
	modelOpen := readUntilAll(t, terminal, []string{
		"model:",
		"command route: model",
		"route source: policy.command",
		"source: app.model",
		"target: primary_model",
		"current primary: opencode-go/deepseek-v4-pro:high",
		"current utility: opencode-go/deepseek-v4-flash:max",
		"codex/codex-high provider=codex",
		"focus: model",
	}, 10*time.Second)
	assertNoModelAutonomySmokeLeaks(t, modelOpen, env, workspace)
	for _, forbidden := range []string{"marketplace", "provider execution", "config path", "config.toml"} {
		if strings.Contains(modelOpen, forbidden) {
			t.Fatalf("model switch PTY output exposed forbidden marker %q: %q", forbidden, modelOpen)
		}
	}

	if _, err := terminal.Write([]byte("\x1b[B\r")); err != nil {
		t.Fatalf("send model down and enter input: %v", err)
	}
	modelApplied := readUntilAll(t, terminal, []string{
		"Model codex/codex-high",
		"current primary: codex/codex-high",
		"detail: applied codex/codex-high to current session; config file unchanged",
	}, 10*time.Second)
	assertNoModelAutonomySmokeLeaks(t, modelApplied, env, workspace)

	if _, err := terminal.Write([]byte("/auto\r")); err != nil {
		t.Fatalf("send /auto command input: %v", err)
	}
	autoOpen := readUntilAll(t, terminal, []string{
		"auto:",
		"command route: auto",
		"source: app.autonomy",
		"current: yolo",
		"levels:",
		"read status=available current=false",
		"yolo status=available current=true",
		"focus: auto",
	}, 10*time.Second)
	assertNoModelAutonomySmokeLeaks(t, autoOpen, env, workspace)

	if _, err := terminal.Write([]byte("\x1b[A\x1b[A\x1b[A\r")); err != nil {
		t.Fatalf("send autonomy up navigation and enter input: %v", err)
	}
	autoApplied := readUntilAll(t, terminal, []string{
		"Auto off",
		"current: off",
		"off status=available current=true",
		"detail: applied off autonomy to current session; config file unchanged",
	}, 10*time.Second)
	assertNoModelAutonomySmokeLeaks(t, autoApplied, env, workspace)

	configPath := filepath.Join(env.xdgConfigHome, "aila", "config.toml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read temp XDG config after model/autonomy smoke: %v", err)
	}
	configText := string(config)
	for _, marker := range []string{`model = "opencode-go/deepseek-v4-pro:high"`, `model = "opencode-go/deepseek-v4-flash:max"`, `level = "yolo"`} {
		if !strings.Contains(configText, marker) {
			t.Fatalf("temp config lost default marker %q after session-scoped switch: %s", marker, configText)
		}
	}
	for _, forbidden := range []string{"codex/codex-high", "level = \"off\""} {
		if strings.Contains(configText, forbidden) {
			t.Fatalf("temp config persisted session selection %q: %s", forbidden, configText)
		}
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after model/autonomy smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("model/autonomy PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("model/autonomy PTY did not clean up before timeout: %v", ctx.Err())
	}

	assertNoDurableStatePollution(t, env, baseline)
}

func assertNoModelAutonomySmokeLeaks(t *testing.T, output string, env ailaPTYTestEnv, workspace string) {
	t.Helper()
	for _, forbidden := range []string{env.home, env.xdgConfigHome, workspace, ".config/aila", "config.toml", "oauth", "password=", "secret=", "token="} {
		if forbidden != "" && strings.Contains(output, forbidden) {
			t.Fatalf("model/autonomy PTY output leaked %q: %q", forbidden, output)
		}
	}
}

func TestSessionCommandFamilyPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 160, 60, env.vars, func(workspace string) {
		seedCurrentSessionSnapshot(t, workspace)
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntilAll(t, terminal, []string{
		"Aila",
		"project store: initialized - project store ready",
		"Prompt",
	}, 20*time.Second)

	if _, err := terminal.Write([]byte("/continue\r")); err != nil {
		t.Fatalf("send /continue command input: %v", err)
	}
	resumed := readUntilAll(t, terminal, []string{
		"session:",
		"command route: continue",
		"route source: policy.command",
		"source: app.session",
		"action: continue",
		"status: loaded",
		"session id: current",
		"memory: visible",
		"detail: restored current session snapshot",
		"> current status=loaded memory=visible detail=current session",
		"focus: session",
		"Resumed memory:",
		"user: remembered smoke prompt",
		"assistant: remembered smoke answer",
	}, 10*time.Second)
	assertNoResumeSmokeLeaks(t, resumed, env, workspace)
	assertCurrentSessionSnapshotExists(t, workspace)

	if _, err := terminal.Write([]byte("\r")); err != nil {
		t.Fatalf("send Enter after session resume: %v", err)
	}

	if _, err := terminal.Write([]byte("/clear\r")); err != nil {
		t.Fatalf("send /clear command input: %v", err)
	}
	cleared := readUntilAll(t, terminal, []string{
		"session:",
		"command route: clear",
		"route source: policy.command",
		"source: app.session",
		"action: clear",
		"status: cleared",
		"session id: current",
		"memory: cleared",
		"detail: cleared visible session and current memory",
		"No messages yet.",
	}, 10*time.Second)
	assertNoResumeSmokeLeaks(t, cleared, env, workspace)
	for _, forbidden := range []string{"remembered smoke prompt", "remembered smoke answer", "Queued input:", "Resumed memory:"} {
		if strings.Contains(cleared, forbidden) {
			t.Fatalf("clear session output retained memory marker %q: %q", forbidden, cleared)
		}
	}
	assertCurrentSessionSnapshotMissing(t, workspace)

	if _, err := terminal.Write([]byte("/new\r")); err != nil {
		t.Fatalf("send /new command input: %v", err)
	}
	fresh := readUntilAll(t, terminal, []string{
		"command route: new",
		"source: app.session",
		"action: new",
		"status: fresh",
		"memory: fresh",
		"detail: started fresh session and preserved project store",
	}, 10*time.Second)
	assertNoResumeSmokeLeaks(t, fresh, env, workspace)
	for _, forbidden := range []string{"remembered smoke prompt", "remembered smoke answer", "Queued input:", "Resumed memory:"} {
		if strings.Contains(fresh, forbidden) {
			t.Fatalf("new session output retained memory marker %q: %q", forbidden, fresh)
		}
	}
	assertFreshCurrentSessionSnapshot(t, workspace)

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after session command smoke: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("session command PTY returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("session command PTY did not clean up before timeout: %v", ctx.Err())
	}

	assertFreshCurrentSessionSnapshot(t, workspace)
	assertNoDurableStatePollution(t, env, baseline)
}

func TestResizePTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	env.vars = append(env.vars, "AILA_FAKE_PROMPT_ECHO=1")
	ctx, cancel, terminal, wait := startAilaPTYWithSizeAndEnv(t, 160, 45, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	startup := readUntil(t, terminal, "160x45", 20*time.Second)
	if !strings.Contains(startup, "Aila") {
		t.Fatalf("wide startup output missing Aila marker: %q", startup)
	}

	if _, err := terminal.Write([]byte("resize smoke\r")); err != nil {
		t.Fatalf("send resize smoke prompt input: %v", err)
	}
	readUntil(t, terminal, "assistant: Fake Aila response: resize smoke", 10*time.Second)

	if err := pty.Setsize(terminal, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("resize PTY to 80x24: %v", err)
	}
	resized := readUntilAll(t, terminal, []string{
		"Aila",
		"80x24",
		"Conversation",
		"user: resize smoke",
		"assistant: Fake Aila response: resize smoke",
		"Prompt",
		">",
		"git: placeholder | context: placeholder | q quit",
	}, 10*time.Second)
	if strings.Contains(resized, "Session") {
		t.Fatalf("80x24 resize output exposed right rail: %q", resized)
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input after resize: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("resize TUI quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("resize TUI did not clean up before timeout: %v", ctx.Err())
	}
}

func TestM8DisplayLabelsPTYSmoke(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping PTY smoke test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	ctx, cancel, terminal, wait := startAilaPTYWithSizeAndEnv(t, 80, 24, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	startup := readUntilAll(t, terminal, []string{
		"Aila",
		"primary model: opencode-go/deepseek-v4-pro:high",
		"utility model: opencode-go/deepseek-v4-flash:max",
		"autonomy: yolo (display-only)",
	}, 20*time.Second)
	for _, forbidden := range []string{"OPENAI", "ANTHROPIC", "GOOGLE_API", "config.toml", ".config/aila"} {
		if strings.Contains(startup, forbidden) {
			t.Fatalf("scrubbed PTY startup exposed config or credential marker %q: %q", forbidden, startup)
		}
	}
	configPath := filepath.Join(env.xdgConfigHome, "aila", "config.toml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read temp XDG config created by PTY startup: %v", err)
	}
	configText := string(config)
	for _, marker := range []string{
		`model = "opencode-go/deepseek-v4-pro:high"`,
		`[llm.utility]`,
		`model = "opencode-go/deepseek-v4-flash:max"`,
		`[autonomy]`,
		`level = "yolo"`,
	} {
		if !strings.Contains(configText, marker) {
			t.Fatalf("temp XDG config missing default marker %q: %s", marker, configText)
		}
	}

	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q quit input: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("display labels TUI quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("display labels TUI did not quit after q: %v", ctx.Err())
	}
}

func startAilaPTY(t *testing.T) (context.Context, context.CancelFunc, *os.File, <-chan error) {
	t.Helper()
	return startAilaPTYWithSize(t, 80, 24)
}

func startAilaPTYWithSize(t *testing.T, cols uint16, rows uint16) (context.Context, context.CancelFunc, *os.File, <-chan error) {
	t.Helper()
	return startAilaPTYWithSizeAndEnv(t, cols, rows, newAilaPTYEnv(t).vars)
}

func startAilaPTYWithSizeAndEnv(t *testing.T, cols uint16, rows uint16, env []string) (context.Context, context.CancelFunc, *os.File, <-chan error) {
	t.Helper()
	ctx, cancel, terminal, wait, _ := startAilaPTYWithSizeEnvAndWorkspace(t, cols, rows, env)
	return ctx, cancel, terminal, wait
}

func startAilaPTYWithSizeEnvAndWorkspace(t *testing.T, cols uint16, rows uint16, env []string) (context.Context, context.CancelFunc, *os.File, <-chan error, string) {
	t.Helper()
	ctx, cancel, terminal, wait, workspace, _ := startAilaPTYWithProcess(t, cols, rows, env)
	return ctx, cancel, terminal, wait, workspace
}

func startAilaPTYWithProcess(t *testing.T, cols uint16, rows uint16, env []string) (context.Context, context.CancelFunc, *os.File, <-chan error, string, *exec.Cmd) {
	t.Helper()
	return startAilaPTYWithArgsAndSetup(t, nil, cols, rows, env, nil)
}

func startAilaPTYWithArgsSizeEnvAndWorkspace(t *testing.T, args []string, cols uint16, rows uint16, env []string, setup func(string)) (context.Context, context.CancelFunc, *os.File, <-chan error, string) {
	t.Helper()
	ctx, cancel, terminal, wait, workspace, _ := startAilaPTYWithArgsAndSetup(t, args, cols, rows, env, setup)
	return ctx, cancel, terminal, wait, workspace
}

func startAilaPTYWithArgsAndSetup(t *testing.T, args []string, cols uint16, rows uint16, env []string, setup func(string)) (context.Context, context.CancelFunc, *os.File, <-chan error, string, *exec.Cmd) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		cancel()
		t.Fatalf("create PTY workspace: %v", err)
	}
	if setup != nil {
		setup(workspace)
	}
	binary := buildAilaTestBinary(t, env, tmp)

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = workspace
	cmd.Env = env

	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		cancel()
		t.Fatalf("start static TUI in PTY: %v", err)
	}

	wait := make(chan error, 1)
	go func() {
		wait <- cmd.Wait()
	}()
	return ctx, cancel, terminal, wait, workspace, cmd
}

func buildAilaTestBinary(t *testing.T, env []string, dir string) string {
	t.Helper()

	if cachedTestBinary != "" {
		return cachedTestBinary
	}

	binary := filepath.Join(dir, "aila")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = mustSourceDir(t)
	build.Env = env
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build aila test binary: %v\n%s", err, output)
	}
	return binary
}

func seedCurrentSessionSnapshot(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open resume smoke project store: %v", err)
	}
	snapshot := state.SessionSnapshot{
		SchemaVersion: state.CurrentSessionSnapshotSchemaVersion,
		SessionID:     "current",
		Runtime: state.SessionSnapshotRuntime{
			Status: "idle",
			Source: "runtime.dispatch",
			Detail: "resumed current session",
			Result: "remembered smoke result",
		},
		Transcript: []state.SessionSnapshotTurn{
			{Role: "user", Source: "prompt", Text: "remembered smoke prompt"},
			{Role: "assistant", Source: "fake-runtime", Text: "remembered smoke answer"},
		},
		Queued: []state.SessionSnapshotQueuedEntry{
			{ID: "queue-1", Source: "prompt", Text: "remembered queued smoke input"},
		},
		Diagnostics: []state.SessionSnapshotDiagnostic{
			{Severity: string(diagnostic.SeverityWarning), Source: string(diagnostic.SourceStateSnapshot), Message: "remembered smoke diagnostic"},
		},
		Blockers: []state.SessionSnapshotBlocker{{Source: "runtime.dispatch", Text: "remembered smoke blocker"}},
		Concerns: []state.SessionSnapshotConcern{{Source: "display.status", Text: "remembered smoke concern"}},
	}
	location, err := store.WriteCurrentSessionSnapshot(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("write resume smoke snapshot: %v", err)
	}
	if filepath.ToSlash(location.Provenance.RelativePath) != "sessions/current.json" {
		t.Fatalf("resume smoke snapshot path = %q, want sessions/current.json", location.Provenance.RelativePath)
	}
}

func seedInteractiveWriteBuildWorkspace(t *testing.T, workspace string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nInteractive write build loop fixture.\n"), 0o644); err != nil {
		t.Fatalf("write interactive build README: %v", err)
	}
	runPTYGit(t, workspace, "init")
	runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "add", "README.md")
	runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
}

func seedNonInteractiveRunSmokeWorkspace(t *testing.T, workspace string) {
	t.Helper()

	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create run smoke workspace: %v", err)
	}
	files := map[string]string{
		"README.md":  "# Run Smoke Repo\n\nA tiny repo for non-interactive inspection.\n",
		"ROADMAP.md": "# Roadmap\n\n- non-interactive read-only run smoke\n",
		"AGENTS.md":  "# Agent Instructions\n\nKeep the run smoke deterministic.\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write run smoke %s: %v", name, err)
		}
	}
	runPTYGit(t, workspace, "init")
}

func seedNonInteractiveWriteRunSmokeWorkspace(t *testing.T, workspace string) {
	t.Helper()

	seedNonInteractiveRunSmokeWorkspace(t, workspace)
	runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "add", "README.md", "ROADMAP.md", "AGENTS.md")
	runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
}

func seedDiffSmokeWorkspace(t *testing.T, workspace string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(workspace, "internal"), 0o755); err != nil {
		t.Fatalf("create diff smoke directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "demo.txt"), []byte("old value\n"), 0o644); err != nil {
		t.Fatalf("write diff smoke base file: %v", err)
	}
	runPTYGit(t, workspace, "init")
	runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "add", "internal/demo.txt")
	runPTYGit(t, workspace, "-c", "user.name=Aila Tests", "-c", "user.email=aila@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
	if err := os.WriteFile(filepath.Join(workspace, "internal", "demo.txt"), []byte("new value\nsecond value\n"), 0o644); err != nil {
		t.Fatalf("write diff smoke changed file: %v", err)
	}
}

func runPTYGit(t *testing.T, workspace string, args ...string) {
	t.Helper()
	runPTYGitOutput(t, workspace, args...)
}

func runPTYGitOutput(t *testing.T, workspace string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = workspace
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func seedFakeHistoryEvents(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open history smoke project store: %v", err)
	}
	for _, event := range []historypkg.FakeEvent{
		fakeHistorySmokeEvent("history-event-1", historypkg.EventKindPrompt, "prompt.submit", "user", "user asked for fake history"),
		fakeHistorySmokeEvent("history-event-2", historypkg.EventKindResponse, "runtime.response", "fake-runtime", "fake response summary"),
		fakeHistorySmokeEvent("history-event-3", historypkg.EventKindCommand, "policy.command", "policy.command", "history command summary token=history-smoke-secret"),
		fakeHistorySmokeEvent("history-event-4", historypkg.EventKindRuntime, "runtime.dispatch", "runtime.dispatch", "runtime idle: smoke complete Authorization: Bearer history-smoke-secret"),
		fakeMutationHistorySmokeEvent(),
	} {
		result, err := store.AppendFakeHistory(context.Background(), event)
		if err != nil {
			t.Fatalf("append history smoke event %q: %v", event.EventID, err)
		}
		if result.State != state.FakeHistoryLoaded || len(result.Diagnostics) != 0 {
			t.Fatalf("append history smoke event %q result = %#v, want loaded without diagnostics", event.EventID, result)
		}
	}
}

func fakeHistorySmokeEvent(eventID string, kind historypkg.EventKind, provenance string, source string, displayText string) historypkg.FakeEvent {
	return historypkg.FakeEvent{
		SchemaVersion: historypkg.FakeEventSchemaVersion,
		Kind:          kind,
		EventID:       eventID,
		RunID:         "history-run",
		SessionID:     "history-session",
		Source:        source,
		Provenance:    provenance,
		DisplayText:   displayText,
	}
}

func fakeMutationHistorySmokeEvent() historypkg.FakeEvent {
	event := fakeHistorySmokeEvent("history-event-5", historypkg.EventKindMutation, "mutation.result", "mutation.tool", "mutation write completed notes.txt approval smoke-approval undo delete_created_file")
	event.Mutation = &historypkg.MutationRecord{
		ToolName:              "write",
		Status:                "completed",
		CommandSource:         "approval-write",
		RequestID:             "smoke-write",
		ApprovalID:            "smoke-approval",
		ApprovalAction:        "approve",
		ChangedPaths:          []string{"notes.txt"},
		RequestedPath:         "notes.txt",
		ExpectedEffect:        "create notes through approval smoke",
		PreviousVersion:       "missing",
		NewVersion:            "sha256:smoke-new-version",
		BytesWritten:          6,
		ResolvedPathAvailable: true,
		DecisionRunID:         "smoke-run",
		DecisionCapability:    "approval-write",
	}
	event.Undo = &historypkg.UndoMetadata{
		Available:       true,
		Action:          "delete_created_file",
		Paths:           []string{"notes.txt"},
		PreviousVersion: "missing",
		NewVersion:      "sha256:smoke-new-version",
	}
	return event
}

func mustSourceDir(t *testing.T) string {
	t.Helper()
	sourceDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve aila source dir: %v", err)
	}
	return sourceDir
}

func assertProjectStoreLayout(t *testing.T, workspace string) {
	t.Helper()
	storeRoot := filepath.Join(workspace, ".aila")
	for _, dir := range []string{storeRoot, filepath.Join(storeRoot, "artifacts"), filepath.Join(storeRoot, "indexes")} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat project store directory %q: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("project store path %q is not a directory", dir)
		}
	}
	content, err := os.ReadFile(filepath.Join(storeRoot, "project.toml"))
	if err != nil {
		t.Fatalf("read project store metadata: %v", err)
	}
	if string(content) != "schema_version = 1\n" {
		t.Fatalf("project store metadata = %q, want schema_version", content)
	}
	assertProjectStoreEntries(t, workspace)
}

func assertProjectStoreEntries(t *testing.T, workspace string) {
	t.Helper()

	storeRoot := filepath.Join(workspace, ".aila")
	entries, err := os.ReadDir(storeRoot)
	if err != nil {
		t.Fatalf("read project store root: %v", err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		got = append(got, name)
	}
	if strings.Join(got, ",") != "artifacts/,indexes/,project.toml" {
		t.Fatalf("project store entries = %v, want artifacts indexes and project.toml only", got)
	}
}

func assertNonInteractiveRunStoreState(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open run smoke project store: %v", err)
	}
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read run smoke current snapshot: %v", err)
	}
	if snapshot.State != state.SessionSnapshotLoaded || snapshot.Snapshot.Run == nil {
		t.Fatalf("run smoke snapshot state = %q run=%#v, want loaded run memory", snapshot.State, snapshot.Snapshot.Run)
	}
	run := snapshot.Snapshot.Run
	if run.Mode != "non_interactive_read_only" || run.Prompt != "explain the repo" || run.Status != "flagged" || !run.StoredSession || !run.StoredHistory {
		t.Fatalf("run smoke snapshot run = %#v", run)
	}
	if len(run.InspectedFiles) < 2 || len(run.Commands) != 2 || len(run.SourceRefs) < 4 {
		t.Fatalf("run smoke snapshot evidence = files=%#v commands=%#v source_refs=%#v", run.InspectedFiles, run.Commands, run.SourceRefs)
	}

	history, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("read run smoke fake history: %v", err)
	}
	if history.State != state.FakeHistoryLoaded || len(history.Events) != 5 {
		t.Fatalf("run smoke history state = %q events=%d, want 5", history.State, len(history.Events))
	}
	joined := make([]string, 0, len(history.Events))
	for _, event := range history.Events {
		joined = append(joined, event.DisplayText)
	}
	encoded := strings.Join(joined, "\n")
	for _, want := range []string{"noninteractive run prompt explain the repo", "Read-only run flagged", "check git status --short --branch completed", "check git diff --stat completed"} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("run smoke history missing %q in %q", want, encoded)
		}
	}
	for _, leaked := range []string{workspace, "run-smoke-secret", "token=", "OPENAI_API_KEY"} {
		if strings.Contains(encoded, leaked) {
			t.Fatalf("run smoke history leaked %q in %q", leaked, encoded)
		}
	}
}

func assertNonInteractiveWriteRunStoreState(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open write run smoke project store: %v", err)
	}
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read write run smoke current snapshot: %v", err)
	}
	if snapshot.State != state.SessionSnapshotLoaded || snapshot.Snapshot.Run == nil {
		t.Fatalf("write run smoke snapshot state = %q run=%#v, want loaded run memory", snapshot.State, snapshot.Snapshot.Run)
	}
	run := snapshot.Snapshot.Run
	if run.Mode != "non_interactive_write" || run.Prompt != "create a note" || run.Status != "flagged" || !run.StoredSession || !run.StoredHistory {
		t.Fatalf("write run smoke snapshot run = %#v", run)
	}
	if len(run.ChangedFiles) != 1 || run.ChangedFiles[0].Path != "docs/aila-run-output.md" || run.Mutation == nil {
		t.Fatalf("write run smoke snapshot mutation evidence = changed=%#v mutation=%#v", run.ChangedFiles, run.Mutation)
	}
	if run.Mutation.ToolName != "write" || run.Mutation.Status != "completed" || run.Mutation.DecisionSource != "autonomy_policy" || run.Mutation.DecisionAutonomy != "yolo" || !run.Mutation.Allowed || !run.Mutation.Automatic || run.Mutation.ApprovalRequired {
		t.Fatalf("write run smoke snapshot mutation = %#v", run.Mutation)
	}

	history, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("read write run smoke fake history: %v", err)
	}
	if history.State != state.FakeHistoryLoaded || len(history.Events) != 6 {
		t.Fatalf("write run smoke history state = %q events=%d, want 6", history.State, len(history.Events))
	}
	last := history.Events[len(history.Events)-1]
	if last.Kind != historypkg.EventKindMutation || last.Mutation == nil || last.Undo == nil {
		t.Fatalf("write run smoke last history event = %#v, want mutation with undo", last)
	}
	if last.Mutation.ToolName != "write" || last.Mutation.Status != "completed" || !reflect.DeepEqual(last.Mutation.ChangedPaths, []string{"docs/aila-run-output.md"}) || !last.Undo.Available || last.Undo.Action != "delete_created_file" {
		t.Fatalf("write run smoke mutation history = mutation=%#v undo=%#v", last.Mutation, last.Undo)
	}
}

func assertNoNonInteractiveRunSmokeLeaks(t *testing.T, output string, env ailaPTYTestEnv, workspace string) {
	t.Helper()

	for _, forbidden := range []string{
		workspace,
		env.home,
		env.xdgConfigHome,
		mustRepositoryRoot(t),
		"/tmp",
		"/home/",
		".aila",
		"sessions/current.json",
		"fake-events.jsonl",
		"project.toml",
		"artifacts/",
		"indexes/",
		"config.toml",
		".config/aila",
		"credential",
		"OPENAI_API_KEY",
		"run-smoke-secret",
		"token=",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("non-interactive run smoke output leaked marker %q: %q", forbidden, output)
		}
	}
}

func assertCurrentSessionSnapshotState(t *testing.T, workspace string) {
	t.Helper()

	storeRoot := filepath.Join(workspace, ".aila")
	for _, dir := range []string{storeRoot, filepath.Join(storeRoot, "artifacts"), filepath.Join(storeRoot, "indexes"), filepath.Join(storeRoot, "sessions")} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat resume smoke project store directory %q: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("resume smoke project store path %q is not a directory", dir)
		}
	}
	assertFileContent(t, filepath.Join(storeRoot, "project.toml"), "schema_version = 1\n")
	if _, err := os.Stat(filepath.Join(storeRoot, "sessions", "current.json")); err != nil {
		t.Fatalf("stat current session snapshot: %v", err)
	}

	entries, err := os.ReadDir(storeRoot)
	if err != nil {
		t.Fatalf("read resume smoke project store root: %v", err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		got = append(got, name)
	}
	if strings.Join(got, ",") != "artifacts/,indexes/,project.toml,sessions/" {
		t.Fatalf("resume smoke project store entries = %v, want artifacts indexes sessions current snapshot only", got)
	}
	sessions, err := os.ReadDir(filepath.Join(storeRoot, "sessions"))
	if err != nil {
		t.Fatalf("read resume smoke sessions dir: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Name() != "current.json" || sessions[0].IsDir() {
		t.Fatalf("resume smoke sessions entries = %v, want current.json only", sessions)
	}
}

func assertCurrentSessionSnapshotExists(t *testing.T, workspace string) {
	t.Helper()

	path := filepath.Join(workspace, ".aila", "sessions", "current.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat current session snapshot: %v", err)
	}
}

func assertCurrentSessionSnapshotMissing(t *testing.T, workspace string) {
	t.Helper()

	path := filepath.Join(workspace, ".aila", "sessions", "current.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("current session snapshot still exists after clear, err=%v", err)
	}
}

func assertFreshCurrentSessionSnapshot(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open project store for fresh session snapshot: %v", err)
	}
	result, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read fresh current session snapshot: %v", err)
	}
	if result.State != state.SessionSnapshotLoaded {
		t.Fatalf("fresh current session snapshot state = %s, want loaded", result.State)
	}
	snapshot := result.Snapshot
	if snapshot.SessionID != "current" {
		t.Fatalf("fresh current session id = %q, want current", snapshot.SessionID)
	}
	if snapshot.Runtime.Status != "idle" || snapshot.Runtime.Source != "app.session" || snapshot.Runtime.Detail != "fresh session started" {
		t.Fatalf("fresh current session runtime = %+v", snapshot.Runtime)
	}
	if len(snapshot.Transcript) != 0 || len(snapshot.Queued) != 0 || len(snapshot.Diagnostics) != 0 || len(snapshot.Blockers) != 0 || snapshot.Run != nil {
		t.Fatalf("fresh current session snapshot retained visible memory: %+v", snapshot)
	}
	for _, concern := range snapshot.Concerns {
		if strings.Contains(concern.Text, "remembered smoke") {
			t.Fatalf("fresh current session concern retained remembered memory: %+v", concern)
		}
	}
}

func assertInspectionCommandFamilyStoreState(t *testing.T, workspace string) {
	t.Helper()

	storeRoot := filepath.Join(workspace, ".aila")
	for _, dir := range []string{storeRoot, filepath.Join(storeRoot, "artifacts"), filepath.Join(storeRoot, "history"), filepath.Join(storeRoot, "indexes"), filepath.Join(storeRoot, "sessions")} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat inspection smoke project store directory %q: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("inspection smoke project store path %q is not a directory", dir)
		}
	}
	assertFileContent(t, filepath.Join(storeRoot, "project.toml"), "schema_version = 1\n")
	if _, err := os.Stat(filepath.Join(storeRoot, "sessions", "current.json")); err != nil {
		t.Fatalf("stat inspection smoke current session snapshot: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(storeRoot, "history", "fake-events.jsonl"))
	if err != nil {
		t.Fatalf("read inspection smoke history JSONL: %v", err)
	}
	encoded := string(content)
	for _, marker := range []string{"history-event-1", "history-event-5", "status via shortcut", "review via slash", "Brief: phase idle", "[secret]"} {
		if !strings.Contains(encoded, marker) {
			t.Fatalf("inspection smoke history missing marker %q: %s", marker, encoded)
		}
	}
	for _, leaked := range []string{"history-smoke-secret", "token=", "Authorization:"} {
		if strings.Contains(encoded, leaked) {
			t.Fatalf("inspection smoke history leaked %q: %s", leaked, encoded)
		}
	}
}

func assertVisionCommandStoreState(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open vision command project store: %v", err)
	}
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read vision command current session snapshot: %v", err)
	}
	if snapshot.State != state.SessionSnapshotLoaded {
		t.Fatalf("vision command snapshot state = %q, want loaded", snapshot.State)
	}
	if snapshot.Snapshot.Runtime.Status != "idle" || !strings.Contains(snapshot.Snapshot.Runtime.Result, "Vision shaped project direction and long-term goals") {
		t.Fatalf("vision command runtime snapshot = %+v", snapshot.Snapshot.Runtime)
	}

	history, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("read vision command fake history: %v", err)
	}
	if history.State != state.FakeHistoryLoaded {
		t.Fatalf("vision command history state = %q, want loaded", history.State)
	}
	var sawVisionCommand, sawVisionRuntime bool
	for _, event := range history.Events {
		if event.Kind == historypkg.EventKindCommand && event.DisplayText == "vision via slash" {
			sawVisionCommand = true
		}
		if event.Kind == historypkg.EventKindRuntime && strings.Contains(event.DisplayText, "Vision shaped project direction and long-term goals") {
			sawVisionRuntime = true
		}
	}
	if !sawVisionCommand || !sawVisionRuntime {
		t.Fatalf("vision command history markers command=%v runtime=%v events=%+v", sawVisionCommand, sawVisionRuntime, history.Events)
	}
}

func assertDiscussCommandStoreState(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open discuss command project store: %v", err)
	}
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read discuss command current session snapshot: %v", err)
	}
	if snapshot.State != state.SessionSnapshotLoaded {
		t.Fatalf("discuss command snapshot state = %q, want loaded", snapshot.State)
	}
	if snapshot.Snapshot.Runtime.Status != "idle" || !strings.Contains(snapshot.Snapshot.Runtime.Result, "Discuss recorded a consequential decision") {
		t.Fatalf("discuss command runtime snapshot = %+v", snapshot.Snapshot.Runtime)
	}

	history, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("read discuss command fake history: %v", err)
	}
	if history.State != state.FakeHistoryLoaded {
		t.Fatalf("discuss command history state = %q, want loaded", history.State)
	}
	var sawDiscussCommand, sawDiscussRuntime bool
	for _, event := range history.Events {
		if event.Kind == historypkg.EventKindCommand && event.DisplayText == "discuss via slash" {
			sawDiscussCommand = true
		}
		if event.Kind == historypkg.EventKindRuntime && strings.Contains(event.DisplayText, "Discuss recorded a consequential decision") {
			sawDiscussRuntime = true
		}
	}
	if !sawDiscussCommand || !sawDiscussRuntime {
		t.Fatalf("discuss command history markers command=%v runtime=%v events=%+v", sawDiscussCommand, sawDiscussRuntime, history.Events)
	}
}

func assertResearchCommandStoreState(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open research command project store: %v", err)
	}
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read research command current session snapshot: %v", err)
	}
	if snapshot.State != state.SessionSnapshotLoaded {
		t.Fatalf("research command snapshot state = %q, want loaded", snapshot.State)
	}
	if snapshot.Snapshot.Runtime.Status != "idle" || !strings.Contains(snapshot.Snapshot.Runtime.Result, "Research folded") {
		t.Fatalf("research command runtime snapshot = %+v", snapshot.Snapshot.Runtime)
	}

	history, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("read research command fake history: %v", err)
	}
	if history.State != state.FakeHistoryLoaded {
		t.Fatalf("research command history state = %q, want loaded", history.State)
	}
	var sawResearchCommand, sawResearchRuntime bool
	for _, event := range history.Events {
		if event.Kind == historypkg.EventKindCommand && event.DisplayText == "research via slash" {
			sawResearchCommand = true
		}
		if event.Kind == historypkg.EventKindRuntime && strings.Contains(event.DisplayText, "Research folded") {
			sawResearchRuntime = true
		}
	}
	if !sawResearchCommand || !sawResearchRuntime {
		t.Fatalf("research command history markers command=%v runtime=%v events=%+v", sawResearchCommand, sawResearchRuntime, history.Events)
	}
}

func assertProfileCommandStoreState(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open profile command project store: %v", err)
	}
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read profile command current session snapshot: %v", err)
	}
	if snapshot.State != state.SessionSnapshotLoaded {
		t.Fatalf("profile command snapshot state = %q, want loaded", snapshot.State)
	}
	if snapshot.Snapshot.Runtime.Status != "idle" || !strings.Contains(snapshot.Snapshot.Runtime.Result, "Profile folded") {
		t.Fatalf("profile command runtime snapshot = %+v", snapshot.Snapshot.Runtime)
	}

	history, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("read profile command fake history: %v", err)
	}
	if history.State != state.FakeHistoryLoaded {
		t.Fatalf("profile command history state = %q, want loaded", history.State)
	}
	var sawProfileCommand, sawProfileRuntime bool
	for _, event := range history.Events {
		if event.Kind == historypkg.EventKindCommand && event.DisplayText == "profile via slash" {
			sawProfileCommand = true
		}
		if event.Kind == historypkg.EventKindRuntime && strings.Contains(event.DisplayText, "Profile folded") {
			sawProfileRuntime = true
		}
	}
	if !sawProfileCommand || !sawProfileRuntime {
		t.Fatalf("profile command history markers command=%v runtime=%v events=%+v", sawProfileCommand, sawProfileRuntime, history.Events)
	}
}

func assertOptimizeCommandStoreState(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open optimize command project store: %v", err)
	}
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read optimize command current session snapshot: %v", err)
	}
	if snapshot.State != state.SessionSnapshotLoaded {
		t.Fatalf("optimize command snapshot state = %q, want loaded", snapshot.State)
	}
	if snapshot.Snapshot.Runtime.Status != "idle" || !strings.Contains(snapshot.Snapshot.Runtime.Result, "Optimize measured render_evidence_seconds") {
		t.Fatalf("optimize command runtime snapshot = %+v", snapshot.Snapshot.Runtime)
	}

	history, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("read optimize command fake history: %v", err)
	}
	if history.State != state.FakeHistoryLoaded {
		t.Fatalf("optimize command history state = %q, want loaded", history.State)
	}
	var sawOptimizeCommand, sawOptimizeRuntime bool
	for _, event := range history.Events {
		if event.Kind == historypkg.EventKindCommand && event.DisplayText == "optimize via slash" {
			sawOptimizeCommand = true
		}
		if event.Kind == historypkg.EventKindRuntime && strings.Contains(event.DisplayText, "Optimize measured render_evidence_seconds") {
			sawOptimizeRuntime = true
		}
	}
	if !sawOptimizeCommand || !sawOptimizeRuntime {
		t.Fatalf("optimize command history markers command=%v runtime=%v events=%+v", sawOptimizeCommand, sawOptimizeRuntime, history.Events)
	}
}

func assertDocumentCommandStoreState(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open document command project store: %v", err)
	}
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read document command current session snapshot: %v", err)
	}
	if snapshot.State != state.SessionSnapshotLoaded {
		t.Fatalf("document command snapshot state = %q, want loaded", snapshot.State)
	}
	if snapshot.Snapshot.Runtime.Status != "idle" || snapshot.Snapshot.Runtime.Detail != "document capability status" || !strings.Contains(snapshot.Snapshot.Runtime.Result, "Document aligned docs/aila-documentation-output.md") {
		t.Fatalf("document command runtime snapshot = %+v", snapshot.Snapshot.Runtime)
	}

	history, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("read document command fake history: %v", err)
	}
	if history.State != state.FakeHistoryLoaded {
		t.Fatalf("document command history state = %q, want loaded", history.State)
	}
	var sawDocumentCommand, sawDocumentRuntime, sawMutation bool
	for _, event := range history.Events {
		if event.Kind == historypkg.EventKindCommand && event.DisplayText == "document via slash" {
			sawDocumentCommand = true
		}
		if event.Kind == historypkg.EventKindRuntime && strings.Contains(event.DisplayText, "Document aligned docs/aila-documentation-output.md") {
			sawDocumentRuntime = true
		}
		if event.Kind == historypkg.EventKindMutation && event.Mutation != nil && event.Mutation.RequestID == "document-alignment" {
			sawMutation = true
			if event.Mutation.ToolName != "write" || event.Mutation.Status != "completed" || !reflect.DeepEqual(event.Mutation.ChangedPaths, []string{"docs/aila-documentation-output.md"}) || event.Mutation.CommandSource != "document" || event.Undo == nil || !event.Undo.Available || event.Undo.Action != "delete_created_file" {
				t.Fatalf("document command mutation history = mutation=%+v undo=%+v", event.Mutation, event.Undo)
			}
		}
	}
	if !sawDocumentCommand || !sawDocumentRuntime || !sawMutation {
		t.Fatalf("document command history markers command=%v runtime=%v mutation=%v events=%+v", sawDocumentCommand, sawDocumentRuntime, sawMutation, history.Events)
	}
}

func assertDesignCommandStoreState(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open design command project store: %v", err)
	}
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read design command current session snapshot: %v", err)
	}
	if snapshot.State != state.SessionSnapshotLoaded {
		t.Fatalf("design command snapshot state = %q, want loaded", snapshot.State)
	}
	if snapshot.Snapshot.Runtime.Status != "idle" || snapshot.Snapshot.Runtime.Detail != "design capability status" || !strings.Contains(snapshot.Snapshot.Runtime.Result, "Design recorded 3 decisions") {
		t.Fatalf("design command runtime snapshot = %+v", snapshot.Snapshot.Runtime)
	}

	history, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("read design command fake history: %v", err)
	}
	if history.State != state.FakeHistoryLoaded {
		t.Fatalf("design command history state = %q, want loaded", history.State)
	}
	var sawDesignCommand, sawDesignRuntime bool
	for _, event := range history.Events {
		if event.Kind == historypkg.EventKindCommand && event.DisplayText == "design via slash" {
			sawDesignCommand = true
		}
		if event.Kind == historypkg.EventKindRuntime && strings.Contains(event.DisplayText, "Design recorded 3 decisions") {
			sawDesignRuntime = true
		}
	}
	if !sawDesignCommand || !sawDesignRuntime {
		t.Fatalf("design command history markers command=%v runtime=%v events=%+v", sawDesignCommand, sawDesignRuntime, history.Events)
	}
}

func assertBuildCommandStoreState(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open build command project store: %v", err)
	}
	snapshot, err := store.ReadCurrentSessionSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read build command current session snapshot: %v", err)
	}
	if snapshot.State != state.SessionSnapshotLoaded {
		t.Fatalf("build command snapshot state = %q, want loaded", snapshot.State)
	}
	if snapshot.Snapshot.Runtime.Status != "idle" || snapshot.Snapshot.Runtime.Detail != "build capability status" || !strings.Contains(snapshot.Snapshot.Runtime.Result, "Build completed one bounded step for plan item implement") {
		t.Fatalf("build command runtime snapshot = %+v", snapshot.Snapshot.Runtime)
	}

	history, err := store.ReadFakeHistory(context.Background())
	if err != nil {
		t.Fatalf("read build command fake history: %v", err)
	}
	if history.State != state.FakeHistoryLoaded {
		t.Fatalf("build command history state = %q, want loaded", history.State)
	}
	var sawBuildCommand, sawBuildRuntime, sawMutation bool
	for _, event := range history.Events {
		if event.Kind == historypkg.EventKindCommand && event.DisplayText == "build via slash" {
			sawBuildCommand = true
		}
		if event.Kind == historypkg.EventKindRuntime && strings.Contains(event.DisplayText, "Build completed one bounded step for plan item implement") {
			sawBuildRuntime = true
		}
		if event.Kind == historypkg.EventKindMutation && event.Mutation != nil && event.Mutation.RequestID == "build-implement" {
			sawMutation = true
			if event.Mutation.ToolName != "write" || event.Mutation.Status != "completed" || !reflect.DeepEqual(event.Mutation.ChangedPaths, []string{"docs/aila-build-output.md"}) || event.Mutation.CommandSource != "build" || event.Undo == nil || !event.Undo.Available || event.Undo.Action != "delete_created_file" {
				t.Fatalf("build command mutation history = mutation=%+v undo=%+v", event.Mutation, event.Undo)
			}
		}
	}
	if !sawBuildCommand || !sawBuildRuntime || !sawMutation {
		t.Fatalf("build command history markers command=%v runtime=%v mutation=%v events=%+v", sawBuildCommand, sawBuildRuntime, sawMutation, history.Events)
	}
}

func assertFakeHistoryProjectStoreState(t *testing.T, workspace string) {
	t.Helper()

	storeRoot := filepath.Join(workspace, ".aila")
	for _, dir := range []string{storeRoot, filepath.Join(storeRoot, "artifacts"), filepath.Join(storeRoot, "history"), filepath.Join(storeRoot, "indexes")} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat history smoke project store directory %q: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("history smoke project store path %q is not a directory", dir)
		}
	}
	assertFileContent(t, filepath.Join(storeRoot, "project.toml"), "schema_version = 1\n")

	entries, err := os.ReadDir(storeRoot)
	if err != nil {
		t.Fatalf("read history smoke project store root: %v", err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		got = append(got, name)
	}
	if strings.Join(got, ",") != "artifacts/,history/,indexes/,project.toml" {
		t.Fatalf("history smoke project store entries = %v, want artifacts history indexes and project.toml only", got)
	}

	historyEntries, err := os.ReadDir(filepath.Join(storeRoot, "history"))
	if err != nil {
		t.Fatalf("read history smoke history dir: %v", err)
	}
	if len(historyEntries) != 1 || historyEntries[0].Name() != "fake-events.jsonl" || historyEntries[0].IsDir() {
		t.Fatalf("history smoke history entries = %v, want fake-events.jsonl only", historyEntries)
	}
	content, err := os.ReadFile(filepath.Join(storeRoot, "history", "fake-events.jsonl"))
	if err != nil {
		t.Fatalf("read history smoke JSONL: %v", err)
	}
	for _, marker := range []string{"history-event-1", "history-event-2", "history-event-3", "history-event-4", "history-event-5", "smoke-approval", "delete_created_file", "[secret]"} {
		if !strings.Contains(string(content), marker) {
			t.Fatalf("history smoke JSONL missing marker %q: %s", marker, content)
		}
	}
	for _, leaked := range []string{"history-smoke-secret", "token=", "Authorization:"} {
		if strings.Contains(string(content), leaked) {
			t.Fatalf("history smoke JSONL leaked %q: %s", leaked, content)
		}
	}
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("content of %q = %q, want %q", path, content, want)
	}
}

func assertUndoRedoRecoveryHistory(t *testing.T, workspace string) {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(workspace, ".aila", "history", "fake-events.jsonl"))
	if err != nil {
		t.Fatalf("read undo/redo recovery history JSONL: %v", err)
	}
	encoded := string(content)
	for _, marker := range []string{
		`"kind":"mutation"`,
		`"kind":"command"`,
		`"kind":"recovery"`,
		`"command":"undo"`,
		`"command":"redo"`,
		`"target_event_id":"current-1"`,
		`"action":"delete_created_file"`,
		`"action":"restore_created_file"`,
		`"redo_available":true`,
		`"redo_available":false`,
		`"redo_content":"approved through pty"`,
	} {
		if !strings.Contains(encoded, marker) {
			t.Fatalf("undo/redo recovery history missing marker %q: %s", marker, encoded)
		}
	}
	for _, leaked := range []string{workspace, "/tmp", "/home/", "token=", "Authorization:"} {
		if strings.Contains(encoded, leaked) {
			t.Fatalf("undo/redo recovery history leaked marker %q: %s", leaked, encoded)
		}
	}
}

func assertNoDiagnosticSmokeLeaks(t *testing.T, output string, env ailaPTYTestEnv, workspace string) {
	t.Helper()

	for _, forbidden := range []string{
		workspace,
		env.home,
		env.xdgConfigHome,
		mustRepositoryRoot(t),
		"/tmp",
		"/home/",
		".aila",
		"project.toml",
		"artifacts/",
		"indexes/",
		"config.toml",
		".config/aila",
		"credential",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("diagnostic smoke output leaked marker %q: %q", forbidden, output)
		}
	}
}

func assertNoResumeSmokeLeaks(t *testing.T, output string, env ailaPTYTestEnv, workspace string) {
	t.Helper()

	for _, forbidden := range []string{
		workspace,
		env.home,
		env.xdgConfigHome,
		mustRepositoryRoot(t),
		"/tmp",
		"/home/",
		".aila",
		"sessions/current.json",
		"current.json",
		"project.toml",
		"artifacts/",
		"indexes/",
		"config.toml",
		".config/aila",
		"credential",
		"OPENAI_API_KEY",
		"token=",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("resume smoke output leaked marker %q: %q", forbidden, output)
		}
	}
}

func assertNoDiffSmokeLeaks(t *testing.T, output string, env ailaPTYTestEnv, workspace string) {
	t.Helper()

	for _, forbidden := range []string{
		workspace,
		env.home,
		env.xdgConfigHome,
		mustRepositoryRoot(t),
		"/tmp",
		"/home/",
		".aila",
		"project.toml",
		"artifacts/",
		"indexes/",
		"config.toml",
		".config/aila",
		"credential",
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
		"GOOGLE_API_KEY",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("diff smoke output leaked marker %q: %q", forbidden, output)
		}
	}
}

func assertNoHistorySmokeLeaks(t *testing.T, output string, env ailaPTYTestEnv, workspace string) {
	t.Helper()

	for _, forbidden := range []string{
		workspace,
		env.home,
		env.xdgConfigHome,
		mustRepositoryRoot(t),
		"/tmp",
		"/home/",
		".aila",
		"history/fake-events.jsonl",
		"fake-events.jsonl",
		"project.toml",
		"artifacts/",
		"indexes/",
		"config.toml",
		".config/aila",
		"credential",
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
		"GOOGLE_API_KEY",
		"internal/",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("history smoke output leaked marker %q: %q", forbidden, output)
		}
	}
}

type durableStateBaseline struct {
	snapshots map[string]durablePathSnapshot
}

type durablePathSnapshot struct {
	exists  bool
	entries map[string]durableEntrySnapshot
}

type durableEntrySnapshot struct {
	kind       string
	mode       fs.FileMode
	size       int64
	digest     string
	linkTarget string
}

func captureDurableStateBaseline(t *testing.T) durableStateBaseline {
	t.Helper()

	paths := []string{
		filepath.Join(mustSourceDir(t), ".aila"),
		filepath.Join(mustRepositoryRoot(t), ".aila"),
	}
	snapshots := make(map[string]durablePathSnapshot, len(paths))
	for _, path := range paths {
		snapshots[path] = snapshotDurablePath(t, path)
	}
	return durableStateBaseline{snapshots: snapshots}
}

func assertNoDurableStatePollution(t *testing.T, env ailaPTYTestEnv, baseline durableStateBaseline) {
	t.Helper()

	for _, path := range []string{
		filepath.Join(env.home, ".aila"),
		filepath.Join(env.home, ".config", "aila"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("diagnostic smoke touched durable state path %q, err=%v", path, err)
		}
	}
	for path, before := range baseline.snapshots {
		after := snapshotDurablePath(t, path)
		if !durablePathSnapshotsEqual(before, after) {
			t.Fatalf("diagnostic smoke modified durable state path %q: before=%s after=%s", path, formatDurablePathSnapshot(before), formatDurablePathSnapshot(after))
		}
	}
}

func snapshotDurablePath(t *testing.T, path string) durablePathSnapshot {
	t.Helper()

	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return durablePathSnapshot{}
	}
	if err != nil {
		t.Fatalf("stat durable state path %q: %v", path, err)
	}
	entries := make(map[string]durableEntrySnapshot)
	if !info.IsDir() {
		entries["."] = snapshotDurableEntry(t, path, info)
		return durablePathSnapshot{exists: true, entries: entries}
	}
	if err := filepath.WalkDir(path, func(entryPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(path, entryPath)
		if err != nil {
			return err
		}
		entries[rel] = snapshotDurableEntry(t, entryPath, info)
		return nil
	}); err != nil {
		t.Fatalf("snapshot durable state path %q: %v", path, err)
	}
	return durablePathSnapshot{exists: true, entries: entries}
}

func snapshotDurableEntry(t *testing.T, path string, info fs.FileInfo) durableEntrySnapshot {
	t.Helper()

	entry := durableEntrySnapshot{mode: info.Mode(), size: info.Size()}
	switch {
	case info.Mode().IsRegular():
		entry.kind = "file"
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read durable state file %q: %v", path, err)
		}
		entry.digest = fmt.Sprintf("%x", sha256.Sum256(content))
	case info.IsDir():
		entry.kind = "dir"
	case info.Mode()&os.ModeSymlink != 0:
		entry.kind = "symlink"
		target, err := os.Readlink(path)
		if err != nil {
			t.Fatalf("read durable state symlink %q: %v", path, err)
		}
		entry.linkTarget = target
	default:
		entry.kind = "other"
	}
	return entry
}

func durablePathSnapshotsEqual(left durablePathSnapshot, right durablePathSnapshot) bool {
	if left.exists != right.exists || len(left.entries) != len(right.entries) {
		return false
	}
	for path, leftEntry := range left.entries {
		if right.entries[path] != leftEntry {
			return false
		}
	}
	return true
}

func formatDurablePathSnapshot(snapshot durablePathSnapshot) string {
	if !snapshot.exists {
		return "absent"
	}
	paths := make([]string, 0, len(snapshot.entries))
	for path := range snapshot.entries {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	parts := make([]string, 0, len(paths))
	for _, path := range paths {
		entry := snapshot.entries[path]
		parts = append(parts, fmt.Sprintf("%s:%s:%s:%d:%s:%s", path, entry.kind, entry.mode, entry.size, entry.digest, entry.linkTarget))
	}
	return strings.Join(parts, ",")
}

func mustRepositoryRoot(t *testing.T) string {
	t.Helper()
	return filepath.Dir(filepath.Dir(mustSourceDir(t)))
}

type ailaPTYTestEnv struct {
	vars          []string
	home          string
	xdgConfigHome string
}

func newAilaPTYEnv(t *testing.T) ailaPTYTestEnv {
	t.Helper()

	tmp := t.TempDir()
	for _, dir := range []string{"home", "xdg-cache", "xdg-config", "go-build", "tmp"} {
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0o755); err != nil {
			t.Fatalf("create PTY test environment directory: %v", err)
		}
	}
	xdgConfigHome := filepath.Join(tmp, "xdg-config")

	env := []string{
		"TERM=xterm-256color",
		"HOME=" + filepath.Join(tmp, "home"),
		"XDG_CONFIG_HOME=" + xdgConfigHome,
		"XDG_CACHE_HOME=" + filepath.Join(tmp, "xdg-cache"),
		"GOCACHE=" + filepath.Join(tmp, "go-build"),
		"TMPDIR=" + filepath.Join(tmp, "tmp"),
		"GOENV=off",
		"AILA_AGENT_RUNNER=fake",
	}
	if path := os.Getenv("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}
	for _, name := range []string{"GOROOT", "GOMODCACHE"} {
		if value := goEnv(t, name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	return ailaPTYTestEnv{vars: env, home: filepath.Join(tmp, "home"), xdgConfigHome: xdgConfigHome}
}

func goEnv(t *testing.T, name string) string {
	t.Helper()

	cmd := exec.Command("go", "env", name)
	cmd.Env = []string{"HOME=" + os.Getenv("HOME")}
	if path := os.Getenv("PATH"); path != "" {
		cmd.Env = append(cmd.Env, "PATH="+path)
	}
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("read go env %s: %v", name, err)
	}
	return strings.TrimSpace(string(output))
}

func readUntil(t *testing.T, reader io.Reader, needle string, timeout time.Duration) string {
	t.Helper()
	return readUntilMatch(t, reader, timeout, func(output string) bool {
		return strings.Contains(output, needle)
	}, needle)
}

func readUntilAll(t *testing.T, reader io.Reader, needles []string, timeout time.Duration) string {
	t.Helper()
	return readUntilMatch(t, reader, timeout, func(output string) bool {
		for _, needle := range needles {
			if !strings.Contains(output, needle) {
				return false
			}
		}
		return true
	}, strings.Join(needles, ", "))
}

func readUntilMatch(t *testing.T, reader io.Reader, timeout time.Duration, match func(string) bool, description string) string {
	t.Helper()

	result := make(chan string, 1)
	failure := make(chan error, 1)
	progress := make(chan string, 1)
	go func() {
		var out strings.Builder
		buf := make([]byte, 1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
				select {
				case progress <- out.String():
				default:
				}
				if match(out.String()) {
					result <- out.String()
					return
				}
			}
			if err != nil {
				failure <- fmt.Errorf("read PTY output: %w", err)
				return
			}
		}
	}()

	var partial string
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case output := <-result:
			return output
		case err := <-failure:
			t.Fatal(err)
		case partial = <-progress:
		case <-timer.C:
			t.Fatalf("timed out waiting for %q; partial output: %q", description, partial)
		}
	}
}

func readRemainingPTYOutput(t *testing.T, reader io.Reader, timeout time.Duration) string {
	t.Helper()

	result := make(chan string, 1)
	go func() {
		var out strings.Builder
		buf := make([]byte, 1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
			}
			if err != nil {
				result <- out.String()
				return
			}
		}
	}()

	select {
	case output := <-result:
		return output
	case <-time.After(timeout):
		t.Fatalf("timed out draining PTY output after process exit")
		return ""
	}
}
