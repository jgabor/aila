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
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/jgabor/aila/internal/app"
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/state"
)

func TestStaticTUISmokeStartupAndQuit(t *testing.T) {
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

func TestM17HistoryViewPTYSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	baseline := captureDurableStateBaseline(t)
	ctx, cancel, terminal, wait, workspace := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 120, 32, env.vars, func(workspace string) {
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
		"entries: 4",
		"selected: 1",
		"m17-run m17-session m17-event-1 prompt user asked for fake history",
		"m17-run m17-session m17-event-2 response fake response summary",
		"m17-run m17-session m17-event-3 command history command summary",
		"m17-run m17-session m17-event-4 runtime runtime idle: smoke complete",
		"selected event id: m17-event-1",
		"selected run id: m17-run",
		"selected session id: m17-session",
		"selected kind: prompt",
		"selected text: user asked for fake history",
	}, 10*time.Second)
	assertNoHistorySmokeLeaks(t, output, env, workspace)
	for _, forbidden := range []string{"undo", "redo", "replay", "token=", "Authorization:", "m17-smoke-secret"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("history PTY output exposed forbidden marker %q: %q", forbidden, output)
		}
	}

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

func TestM17HistoryEmptyPTYSmoke(t *testing.T) {
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
	for _, forbidden := range []string{"entries:", "selected event id:", "undo", "redo", "replay"} {
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

func TestPromptSubmitPTYSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	ctx, cancel, terminal, wait := startAilaPTYWithSizeAndEnv(t, 80, 24, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntil(t, terminal, "Aila", 20*time.Second)
	if _, err := terminal.Write([]byte("explain this repo\r")); err != nil {
		t.Fatalf("send prompt submit input: %v", err)
	}

	output := readUntilAll(t, terminal, []string{
		"Stage IDLE | Runtime idle",
		"Runtime status:",
		"status source: runtime.dispatch",
		"detail: fake in-memory runtime loop",
		"active: false",
		"result: Fake Aila response: explain this repo",
		"user: explain this repo",
		"assistant: Fake Aila response: explain this repo",
	}, 10*time.Second)
	for _, forbidden := range []string{"OPENAI", "ANTHROPIC", "GOOGLE_API", "credential", "provider", "tool execution", "config.toml", ".config/aila"} {
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

func TestM24AgentReadOnlyTurnPTYSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	env.vars = append(env.vars, "AILA_AGENT_READONLY=1")
	ctx, cancel, terminal, wait, _ := startAilaPTYWithArgsSizeEnvAndWorkspace(t, nil, 160, 45, env.vars, func(workspace string) {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Aila\nAila is a bounded read-only coding agent.\n"), 0o644); err != nil {
			t.Fatalf("seed README for M24 PTY smoke: %v", err)
		}
	})
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntil(t, terminal, "Aila", 20*time.Second)
	if _, err := terminal.Write([]byte("summarize read only turn\r")); err != nil {
		t.Fatalf("send M24 prompt input: %v", err)
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
		t.Fatalf("M24 PTY smoke exposed out-of-scope approval/mutation text: %q", output)
	}
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after M24 prompt: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("M24 read-only PTY quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("M24 read-only PTY did not quit cleanly: %v", ctx.Err())
	}
}

func TestM24ProviderFailurePTYSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	env := newAilaPTYEnv(t)
	env.vars = append(env.vars, "AILA_AGENT_READONLY=1", "AILA_AGENT_FAILURE=provider_auth_failed")
	ctx, cancel, terminal, wait := startAilaPTYWithSizeAndEnv(t, 160, 45, env.vars)
	defer cancel()
	defer func() { _ = terminal.Close() }()

	readUntil(t, terminal, "Aila", 20*time.Second)
	if _, err := terminal.Write([]byte("trigger provider failure\r")); err != nil {
		t.Fatalf("send M24 provider failure input: %v", err)
	}

	output := readUntilAll(t, terminal, []string{
		"provider_auth_failed: provider authentication failed",
		"source: provider",
		"affected artifact: provider_request",
		"assistant: provider authentication failed",
	}, 10*time.Second)
	if strings.Contains(output, "api_key=") || strings.Contains(output, "Authorization") {
		t.Fatalf("M24 provider failure PTY leaked credential-shaped text: %q", output)
	}
	if _, err := terminal.Write([]byte("q")); err != nil {
		t.Fatalf("send q after M24 provider failure: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("M24 provider failure PTY quit returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("M24 provider failure PTY did not quit cleanly: %v", ctx.Err())
	}
}

func TestM13SubmitWhileActivePTYSmoke(t *testing.T) {
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

func TestM14InterruptActiveWorkPTYSmoke(t *testing.T) {
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

	if _, err := terminal.Write([]byte{0x18, 'c'}); err != nil {
		t.Fatalf("send ctrl+x c interrupt input: %v", err)
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

	if _, err := terminal.Write([]byte{0x18, 'c'}); err != nil {
		t.Fatalf("send second ctrl+x c fake interrupt resolution input: %v", err)
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

func TestM5CommandPTYSmoke(t *testing.T) {
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

	if _, err := terminal.Write([]byte{0x18, 's'}); err != nil {
		t.Fatalf("send ctrl+x s command input: %v", err)
	}
	shortcutStatus := readUntil(t, terminal, "real status sources: deferred", 10*time.Second)
	if !strings.Contains(startup+shortcutStatus, "80x24") {
		t.Fatalf("PTY smoke did not observe fixed-size marker: startup=%q status=%q", startup, shortcutStatus)
	}
	for _, marker := range []string{
		"status:",
		"command route: status",
		"route source: policy.command",
		"Deterministic placeholder status.",
		"real status sources: deferred",
	} {
		if !strings.Contains(shortcutStatus, marker) {
			t.Fatalf("ctrl+x s output missing explicit marker %q: %q", marker, shortcutStatus)
		}
	}

	if _, err := terminal.Write([]byte("/help\r\n")); err != nil {
		t.Fatalf("send /help command input: %v", err)
	}
	readUntil(t, terminal, "Deterministic placeholder help.", 10*time.Second)

	if _, err := terminal.Write([]byte("/status\r\n")); err != nil {
		t.Fatalf("send /status command input: %v", err)
	}
	status := readUntil(t, terminal, "real status sources: deferred", 10*time.Second)
	for _, marker := range []string{
		"status:",
		"command route: status",
		"Deterministic placeholder status.",
		"real status sources: deferred",
	} {
		if !strings.Contains(status, marker) {
			t.Fatalf("/status output missing explicit marker %q: %q", marker, status)
		}
	}

	if _, err := terminal.Write([]byte("/quit\r\n")); err != nil {
		t.Fatalf("send /quit command input: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("/quit command route returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("/quit command route did not clean up before timeout: %v", ctx.Err())
	}
}

func TestM6ResizePTYSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY smoke uses Unix pseudo-terminals")
	}

	ctx, cancel, terminal, wait := startAilaPTYWithSize(t, 160, 45)
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

func seedFakeHistoryEvents(t *testing.T, workspace string) {
	t.Helper()

	store, err := state.OpenProjectStore(context.Background(), workspace)
	if err != nil {
		t.Fatalf("open history smoke project store: %v", err)
	}
	for _, event := range []history.FakeEvent{
		fakeHistorySmokeEvent("m17-event-1", history.EventKindPrompt, "prompt.submit", "user", "user asked for fake history"),
		fakeHistorySmokeEvent("m17-event-2", history.EventKindResponse, "runtime.response", "fake-runtime", "fake response summary"),
		fakeHistorySmokeEvent("m17-event-3", history.EventKindCommand, "policy.command", "policy.command", "history command summary token=m17-smoke-secret"),
		fakeHistorySmokeEvent("m17-event-4", history.EventKindRuntime, "runtime.dispatch", "runtime.dispatch", "runtime idle: smoke complete Authorization: Bearer m17-smoke-secret"),
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

func fakeHistorySmokeEvent(eventID string, kind history.EventKind, provenance string, source string, displayText string) history.FakeEvent {
	return history.FakeEvent{
		SchemaVersion: history.FakeEventSchemaVersion,
		Kind:          kind,
		EventID:       eventID,
		RunID:         "m17-run",
		SessionID:     "m17-session",
		Source:        source,
		Provenance:    provenance,
		DisplayText:   displayText,
	}
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
	for _, marker := range []string{"m17-event-1", "m17-event-2", "m17-event-3", "m17-event-4", "[secret]"} {
		if !strings.Contains(string(content), marker) {
			t.Fatalf("history smoke JSONL missing marker %q: %s", marker, content)
		}
	}
	for _, leaked := range []string{"m17-smoke-secret", "token=", "Authorization:"} {
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
