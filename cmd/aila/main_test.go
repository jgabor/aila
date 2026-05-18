package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jgabor/aila/internal/app"
	"github.com/jgabor/aila/internal/diagnostic"
)

const wantRunOutput = "aila test-version\n" +
	"command: run\n" +
	"mode: non_interactive_read_only\n" +
	"status: completed\n" +
	"prompt: injected prompt\n" +
	"inspected_files:\n" +
	"- README.md status=completed source_ref=README.md:1-20\n" +
	"commands_run:\n" +
	"- git status --short --branch status=completed exit=0 summary=## main\n" +
	"blockers:\n" +
	"- none\n" +
	"caveats:\n" +
	"- none\n" +
	"source_refs:\n" +
	"- README.md:1-20\n" +
	"stored_session: true\n" +
	"stored_history: true\n"

const wantContinueStub = ""

const wantConfigOutput = "path: /tmp/aila-test/config.toml\n" +
	"deferred: interactive config UI\n"

const wantConfigAllOutput = "path: /tmp/aila-test/config.toml\n" +
	"llm.model: test/primary:high\n" +
	"llm.utility.model: test/utility:max\n" +
	"autonomy.level: test-yolo\n"

const wantModelsOutput = "aila test-version\n" +
	"command: models\n" +
	"status: fake diagnostics\n" +
	"filters: none\n" +
	"columns: provider model family class status error\n" +
	"codex codex-high device_code reasoning available -\n" +
	"codex codex-low device_code utility available -\n" +
	"copilot copilot-chat device_code general available -\n" +
	"copilot copilot-fast device_code utility available -\n" +
	"custom deepseek-chat custom general unavailable provider unavailable\n" +
	"custom deepseek-reasoner custom reasoning available -\n" +
	"custom local-chat custom general available -\n" +
	"openai gpt-4.1 api_key general degraded readiness timeout\n" +
	"openai gpt-4.1-mini api_key utility available -\n" +
	"openai o4-mini api_key reasoning available -\n" +
	"opencode-go deepseek-v4-flash api_key utility available -\n" +
	"opencode-go deepseek-v4-pro api_key reasoning available -\n" +
	"opencode-zen zen-flash api_key utility available -\n" +
	"opencode-zen zen-pro api_key general available -\n" +
	"xiaomi-plan mi-flash device_code utility available -\n" +
	"xiaomi-plan mi-pro device_code general available -\n" +
	"zai-plan glm-4.5 device_code reasoning available -\n" +
	"zai-plan glm-4.5-air device_code utility available -\n" +
	"count: 18\n" +
	"source: deterministic-fakes\n"

const wantOpenAIModelsOutput = "aila test-version\n" +
	"command: models\n" +
	"status: fake diagnostics\n" +
	"filters: provider=openai,gpt\n" +
	"columns: provider model family class status error\n" +
	"openai gpt-4.1 api_key general degraded readiness timeout\n" +
	"openai gpt-4.1-mini api_key utility available -\n" +
	"count: 2\n" +
	"source: deterministic-fakes\n"

const wantHelpOutput = "aila test-version\n" +
	"accepted command shape:\n" +
	"  aila run [prompt...] [--model MODEL] [--debug]\n" +
	"  aila continue | aila --continue | aila -c\n" +
	"  aila config [--all] [--debug]\n" +
	"  aila models [filter...] [--debug]\n" +
	"  aila help\n" +
	"  aila --version | aila -V\n" +
	"  aila --debug\n" +
	"Deferred beyond current read-only run: stdin review, session discovery UI, credentials, provider model turns, write tools, workflow transitions, and mutation persistence.\n"

const validCLIFlagsWithDebug = "--model, -m, --continue, -c, --version, -V, --debug"

func TestMainPackageCompiles(t *testing.T) {
	t.Parallel()
}

func TestCLIRunnerNoArgsStartsInteractivePath(t *testing.T) {
	t.Parallel()

	input := strings.NewReader("interactive input")
	var output bytes.Buffer
	var errors bytes.Buffer
	called := false
	runner := cliRunner{
		input:   input,
		output:  &output,
		errors:  &errors,
		version: "test-version",
		start: func(ctx context.Context, gotInput io.Reader, gotOutput io.Writer) error {
			called = true
			if err := ctx.Err(); err != nil {
				t.Fatalf("unexpected canceled context: %v", err)
			}
			if gotInput != input {
				t.Fatal("interactive path did not receive injected input")
			}
			if gotOutput != &output {
				t.Fatal("interactive path did not receive injected output")
			}
			return nil
		},
	}

	if err := runner.run(context.Background(), nil); err != nil {
		t.Fatalf("run no-arg CLI: %v", err)
	}
	if !called {
		t.Fatal("no-argument CLI did not start the interactive path")
	}
	if output.Len() != 0 || errors.Len() != 0 {
		t.Fatalf("no-argument runner wrote CLI output instead of delegating: stdout=%q stderr=%q", output.String(), errors.String())
	}
}

func TestCLIRunnerArgsAreNonInteractiveAndInjected(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	var errors bytes.Buffer
	runner := cliRunner{
		input:   failReader{t: t},
		output:  &output,
		errors:  &errors,
		version: "test-version",
		start: func(context.Context, io.Reader, io.Writer) error {
			t.Fatal("command arguments must not start the interactive TUI path")
			return nil
		},
		runCmd: testRunOutput,
	}

	done := make(chan error, 1)
	go func() {
		done <- runner.run(context.Background(), []string{"run"})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run command-arg CLI: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("command-arg CLI did not return without terminal input")
	}
	if !strings.Contains(output.String(), "test-version") {
		t.Fatalf("command output did not use injected version: %q", output.String())
	}
	if !strings.Contains(output.String(), "mode: non_interactive_read_only") {
		t.Fatalf("command output did not stay within the read-only run boundary: %q", output.String())
	}
	if errors.Len() != 0 {
		t.Fatalf("command-arg CLI wrote unexpected stderr: %q", errors.String())
	}
}

func TestCLIRunnerRecognizesSupportedCommands(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"run":      "mode: non_interactive_read_only",
		"continue": "",
		"config":   "deferred: interactive config UI",
		"models":   "status: fake diagnostics",
		"help":     "accepted command shape:",
	}

	for command, want := range tests {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := runCLITest(t, []string{command})
			if err != nil {
				t.Fatalf("run %s command: %v", command, err)
			}
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout did not recognize %s command: %q", command, stdout)
			}
			if stderr != "" {
				t.Fatalf("stderr for accepted command: %q", stderr)
			}
		})
	}
}

func TestCLIRunnerContinueShapesStartResumePath(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"continue"}, {"--continue"}, {"-c"}} {
		args := args
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()

			input := strings.NewReader("resume input")
			var output bytes.Buffer
			var errors bytes.Buffer
			called := false
			runner := cliRunner{
				input:   input,
				output:  &output,
				errors:  &errors,
				version: "test-version",
				start: func(context.Context, io.Reader, io.Writer) error {
					t.Fatal("continue must not use normal startup")
					return nil
				},
				resume: func(ctx context.Context, gotInput io.Reader, gotOutput io.Writer) error {
					called = true
					if err := ctx.Err(); err != nil {
						t.Fatalf("unexpected canceled context: %v", err)
					}
					if gotInput != input || gotOutput != &output {
						t.Fatal("resume path did not receive injected streams")
					}
					return nil
				},
			}

			if err := runner.run(context.Background(), args); err != nil {
				t.Fatalf("run continue args %v: %v", args, err)
			}
			if !called {
				t.Fatalf("continue args %v did not start resume path", args)
			}
			if output.Len() != 0 || errors.Len() != 0 {
				t.Fatalf("continue wrote CLI stub output: stdout=%q stderr=%q", output.String(), errors.String())
			}
		})
	}
}

func TestCLIRunnerAcceptsGlobalFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "long model", args: []string{"--model", "openai/gpt", "run"}, want: wantRunOutput},
		{name: "short model", args: []string{"-m", "openai/gpt", "run"}, want: wantRunOutput},
		{name: "equals model", args: []string{"--model=openai/gpt", "run"}, want: wantRunOutput},
		{name: "long continue", args: []string{"--continue"}, want: wantContinueStub},
		{name: "short continue", args: []string{"-c"}, want: wantContinueStub},
		{name: "long version", args: []string{"--version"}, want: "aila test-version\n"},
		{name: "short version", args: []string{"-V"}, want: "aila test-version\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := runCLITest(t, test.args)
			if err != nil {
				t.Fatalf("run args %v: %v", test.args, err)
			}
			if stdout != test.want {
				t.Fatalf("stdout mismatch for %v: got %q want %q", test.args, stdout, test.want)
			}
			if stderr != "" {
				t.Fatalf("stderr for accepted flags: %q", stderr)
			}
		})
	}
}

func TestCLICommandShapesExitWithExpectedStreams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "run", args: []string{"run"}, wantStdout: wantRunOutput},
		{name: "run global model flag", args: []string{"--model", "openai/gpt", "run"}, wantStdout: wantRunOutput},
		{name: "run short model flag", args: []string{"-m", "openai/gpt", "run"}, wantStdout: wantRunOutput},
		{name: "run equals model flag", args: []string{"--model=openai/gpt", "run"}, wantStdout: wantRunOutput},
		{name: "run prompt model", args: []string{"run", "write", "tests", "--model", "openai/gpt"}, wantStdout: wantRunOutput},
		{name: "continue command", args: []string{"continue"}, wantStdout: wantContinueStub},
		{name: "continue flag", args: []string{"--continue"}, wantStdout: wantContinueStub},
		{name: "short continue flag", args: []string{"-c"}, wantStdout: wantContinueStub},
		{name: "config", args: []string{"config"}, wantStdout: wantConfigOutput},
		{name: "config all", args: []string{"config", "--all"}, wantStdout: wantConfigAllOutput},
		{name: "models", args: []string{"models"}, wantStdout: wantModelsOutput},
		{name: "models filter", args: []string{"models", "provider=openai", "gpt"}, wantStdout: wantOpenAIModelsOutput},
		{name: "help", args: []string{"help"}, wantStdout: wantHelpOutput},
		{name: "version", args: []string{"--version"}, wantStdout: "aila test-version\n"},
		{name: "short version", args: []string{"-V"}, wantStdout: "aila test-version\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, code := runCLIExitTest(t, test.args)
			if code != test.wantCode || stdout != test.wantStdout || stderr != test.wantStderr {
				t.Fatalf("run %v: code=%d stdout=%q stderr=%q, want code=%d stdout=%q stderr=%q", test.args, code, stdout, stderr, test.wantCode, test.wantStdout, test.wantStderr)
			}
		})
	}
}

func TestCLIRunnerRejectsUnknownCommandsAndFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown command", args: []string{"status"}, want: "unknown command \"status\"; valid CLI commands: run, continue, config, models, help"},
		{name: "unknown positional after help", args: []string{"help", "topic"}, want: "unknown command \"topic\"; valid CLI commands: run, continue, config, models, help"},
		{name: "unknown flag", args: []string{"run", "--dry-run"}, want: "unknown flag \"--dry-run\"; valid CLI flags: " + validCLIFlagsWithDebug},
		{name: "missing model value", args: []string{"--model"}, want: "missing value for --model; valid CLI flags: " + validCLIFlagsWithDebug},
		{name: "missing command", args: []string{"--model", "openai/gpt"}, want: "missing command; valid CLI commands: run, continue, config, models, help"},
		{name: "config all on models", args: []string{"models", "--all"}, want: "unsupported CLI flag \"--all\" for models; valid config shape: config [--all]"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := runCLITest(t, test.args)
			if err == nil {
				t.Fatalf("expected parse failure for %v", test.args)
			}
			if err.Error() != test.want {
				t.Fatalf("error mismatch for %v: got %q want %q", test.args, err.Error(), test.want)
			}
			if stdout != "" || stderr != "" {
				t.Fatalf("parse failure wrote output: stdout=%q stderr=%q", stdout, stderr)
			}
		})
	}
}

func TestCLIUnknownInputsReturnBoundedDiagnostics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown command", args: []string{"status"}, want: "unknown command \"status\"; valid CLI commands: run, continue, config, models, help"},
		{name: "unknown flag", args: []string{"run", "--dry-run"}, want: "unknown flag \"--dry-run\"; valid CLI flags: " + validCLIFlagsWithDebug},
		{name: "unknown short flag", args: []string{"run", "-x"}, want: "unknown flag \"-x\"; valid CLI flags: " + validCLIFlagsWithDebug},
		{name: "missing model value", args: []string{"--model"}, want: "missing value for --model; valid CLI flags: " + validCLIFlagsWithDebug},
		{name: "empty model value", args: []string{"--model="}, want: "missing value for --model; valid CLI flags: " + validCLIFlagsWithDebug},
		{name: "missing command", args: []string{"--model", "openai/gpt"}, want: "missing command; valid CLI commands: run, continue, config, models, help"},
		{name: "ambiguous commands", args: []string{"run", "config"}, want: "incompatible CLI commands \"run\" and \"config\"; valid CLI commands: run, continue, config, models, help"},
		{name: "ambiguous continuation", args: []string{"run", "--continue"}, want: "incompatible CLI continuation shape \"run\" with --continue; use continue or --continue, not run --continue"},
		{name: "command specific flag", args: []string{"models", "--all"}, want: "unsupported CLI flag \"--all\" for models; valid config shape: config [--all]"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, code := runCLIExitTest(t, test.args)
			wantStderr := test.want + "\n"
			if code != 1 || stdout != "" || stderr != wantStderr {
				t.Fatalf("run %v: code=%d stdout=%q stderr=%q, want code=1 stdout=%q stderr=%q", test.args, code, stdout, stderr, "", wantStderr)
			}
			if len(stderr) > 240 {
				t.Fatalf("diagnostic for %v is not bounded: %d bytes: %q", test.args, len(stderr), stderr)
			}
		})
	}
}

func TestShutdownErrorExitsCleanlyWithDiagnostic(t *testing.T) {
	t.Parallel()

	shutdown := app.NewShutdownError([]diagnostic.Diagnostic{diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategorySignalShutdown,
		Source:           diagnostic.SourceSignal,
		Severity:         diagnostic.SeverityWarning,
		Message:          "signal-triggered shutdown requested: context canceled",
		AffectedArtifact: diagnostic.ArtifactRuntimeEffect,
		RecoveryAction:   diagnostic.RecoveryIgnoreForRun,
		UserInputNeeded:  false,
	})})
	var errors bytes.Buffer

	code := exitCodeForError(shutdown, &errors)

	if code != 0 {
		t.Fatalf("exit code = %d, want clean shutdown", code)
	}
	if got := errors.String(); !strings.Contains(got, "shutdown: signal_shutdown: signal-triggered shutdown requested") {
		t.Fatalf("stderr = %q, want shutdown diagnostic", got)
	}
}

func TestCLIRunnerContinuationShapesAreDeterministic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantOut string
		wantErr string
	}{
		{name: "continue command", args: []string{"continue"}, wantOut: wantContinueStub},
		{name: "continue flag", args: []string{"--continue"}, wantOut: wantContinueStub},
		{name: "short continue flag", args: []string{"-c"}, wantOut: wantContinueStub},
		{name: "run continue flag", args: []string{"run", "--continue"}, wantErr: "incompatible CLI continuation shape \"run\" with --continue; use continue or --continue, not run --continue"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := runCLITest(t, test.args)
			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("expected parse failure for %v", test.args)
				}
				if err.Error() != test.wantErr {
					t.Fatalf("error mismatch: got %q want %q", err.Error(), test.wantErr)
				}
				if stdout != "" || stderr != "" {
					t.Fatalf("parse failure wrote output: stdout=%q stderr=%q", stdout, stderr)
				}
				return
			}

			if err != nil {
				t.Fatalf("run continuation shape %v: %v", test.args, err)
			}
			if stdout != test.wantOut {
				t.Fatalf("stdout mismatch: got %q want %q", stdout, test.wantOut)
			}
			if stderr != "" {
				t.Fatalf("stderr for accepted continuation shape: %q", stderr)
			}
		})
	}
}

func TestCLIRunnerRunCommandUsesInjectedReadOnlyRunner(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runCLITest(t, []string{"run", "write", "code"})
	if err != nil {
		t.Fatalf("run prompt stub: %v", err)
	}
	if stdout != wantRunOutput {
		t.Fatalf("stdout mismatch: got %q want %q", stdout, wantRunOutput)
	}
	if stderr != "" {
		t.Fatalf("stderr for run prompt stub: %q", stderr)
	}
}

func TestCLIRunnerConfigCommandReportsPathAndDefersUI(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	stdout, stderr, err := runCLIWithConfigCommand(t, []string{"config"})
	if err != nil {
		t.Fatalf("run config command: %v", err)
	}
	wantPath := filepath.Join(configHome, "aila", "config.toml")
	want := "path: " + wantPath + "\n" +
		"deferred: interactive config UI\n"
	if stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr for config command: %q", stderr)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("created config stat: %v", err)
	}
}

func TestCLIRunnerConfigAllCreatesDefaultsAndPrintsBoundedValues(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "xdg")
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", home)

	done := make(chan error, 1)
	var stdout string
	var stderr string
	go func() {
		var err error
		stdout, stderr, err = runCLIWithConfigCommand(t, []string{"config", "--all"})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run config --all: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("config --all did not return without hanging")
	}

	wantPath := filepath.Join(configHome, "aila", "config.toml")
	want := "path: " + wantPath + "\n" +
		"llm.model: opencode-go/deepseek-v4-pro:high\n" +
		"llm.utility.model: opencode-go/deepseek-v4-flash:max\n" +
		"autonomy.level: yolo\n"
	if stdout != want {
		t.Fatalf("stdout mismatch:\ngot  %q\nwant %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr for config --all: %q", stderr)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("created config stat: %v", err)
	}
	if lineCount := strings.Count(stdout, "\n"); lineCount != 4 {
		t.Fatalf("config --all output line count = %d, want 4: %q", lineCount, stdout)
	}
}

func TestCLIRunnerModelsPrintsFakeDiagnostics(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runCLITest(t, []string{"models"})
	if err != nil {
		t.Fatalf("run models: %v", err)
	}
	if stdout != wantModelsOutput {
		t.Fatalf("stdout mismatch: got %q want %q", stdout, wantModelsOutput)
	}
	if stderr != "" {
		t.Fatalf("stderr for models: %q", stderr)
	}
}

func TestCLIRunnerModelsFiltersFakeDiagnostics(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runCLITest(t, []string{"models", "provider=openai", "gpt"})
	if err != nil {
		t.Fatalf("run filtered models: %v", err)
	}
	if stdout != wantOpenAIModelsOutput {
		t.Fatalf("stdout mismatch: got %q want %q", stdout, wantOpenAIModelsOutput)
	}
	if stderr != "" {
		t.Fatalf("stderr for filtered models: %q", stderr)
	}
}

func TestCLIRunnerModelsOutputIsBoundedAndSecretFree(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "secret-from-env")
	t.Setenv("OPENCODE_API_KEY", "other-secret-from-env")

	stdout, stderr, err := runCLITest(t, []string{"models", "status=unavailable"})
	if err != nil {
		t.Fatalf("run unavailable models: %v", err)
	}
	for _, forbidden := range []string{"secret-from-env", "other-secret-from-env"} {
		if strings.Contains(stdout, forbidden) || strings.Contains(stderr, forbidden) {
			t.Fatalf("models output leaked secret %q: stdout=%q stderr=%q", forbidden, stdout, stderr)
		}
	}
	if !strings.Contains(stdout, "custom deepseek-chat custom general unavailable provider unavailable") {
		t.Fatalf("models output missing unavailable fake row: %q", stdout)
	}
	if len(stdout) > 4096 {
		t.Fatalf("models output is not bounded: %d bytes", len(stdout))
	}
	if stderr != "" {
		t.Fatalf("stderr for unavailable models: %q", stderr)
	}
}

func TestCLIRunnerDebugOutputsStructuredDiagnostics(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	var errors bytes.Buffer
	runner := cliRunner{
		input:   failReader{t: t},
		output:  &output,
		errors:  &errors,
		version: "test-version",
		start: func(context.Context, io.Reader, io.Writer) error {
			t.Fatal("debug command must not start the interactive TUI path")
			return nil
		},
		debug: func(context.Context) (string, error) {
			return "{\n  \"diagnostics\": [],\n  \"count\": 0,\n  \"max_count\": 8,\n  \"max_message_bytes\": 240,\n  \"max_output_bytes\": 8192\n}\n", nil
		},
	}

	if err := runner.run(context.Background(), []string{"run", "--debug"}); err != nil {
		t.Fatalf("run debug command: %v", err)
	}
	if errors.Len() != 0 {
		t.Fatalf("stderr for debug command: %q", errors.String())
	}
	stdout := output.String()
	for _, field := range []string{"\"diagnostics\"", "\"count\"", "\"max_count\"", "\"max_message_bytes\"", "\"max_output_bytes\""} {
		if !strings.Contains(stdout, field) {
			t.Fatalf("debug output missing %s: %q", field, stdout)
		}
	}
}

func TestCLIRunnerDebugOnlyReportsEmptyDiagnosticsWithoutInteractiveStartup(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runCLITestWithDebug(t, []string{"--debug"}, "{\n  \"diagnostics\": [],\n  \"count\": 0,\n  \"max_count\": 8,\n  \"max_message_bytes\": 240,\n  \"max_output_bytes\": 8192\n}\n")
	if err != nil {
		t.Fatalf("run debug-only command: %v", err)
	}
	if !strings.Contains(stdout, "\"diagnostics\": []") || !strings.Contains(stdout, "\"count\": 0") {
		t.Fatalf("debug-only output did not report empty set: %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr for debug-only command: %q", stderr)
	}
}

func TestCLIRunnerHelpAndVersionAreStable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "help", args: []string{"help"}, want: wantHelpOutput},
		{name: "version", args: []string{"--version"}, want: "aila test-version\n"},
		{name: "short version", args: []string{"-V"}, want: "aila test-version\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := runCLITest(t, test.args)
			if err != nil {
				t.Fatalf("run stable command %v: %v", test.args, err)
			}
			if stdout != test.want {
				t.Fatalf("stdout mismatch: got %q want %q", stdout, test.want)
			}
			if stderr != "" {
				t.Fatalf("stderr for stable command: %q", stderr)
			}
		})
	}
}

func TestCLIRunnerUsesInjectedErrorOutput(t *testing.T) {
	t.Parallel()

	var errorsBuffer bytes.Buffer
	runner := cliRunner{
		input:   failReader{t: t},
		output:  failWriter{},
		errors:  &errorsBuffer,
		version: "test-version",
		start: func(context.Context, io.Reader, io.Writer) error {
			t.Fatal("command arguments must not start the interactive TUI path")
			return nil
		},
		runCmd: testRunOutput,
	}

	if err := runner.run(context.Background(), []string{"run"}); err == nil {
		t.Fatal("expected output write failure")
	}
	if !strings.Contains(errorsBuffer.String(), "write CLI output") {
		t.Fatalf("runner did not use injected error output: %q", errorsBuffer.String())
	}
}

func runCLITest(t *testing.T, args []string) (string, string, error) {
	t.Helper()

	var output bytes.Buffer
	var errors bytes.Buffer
	runner := cliRunner{
		input:   failReader{t: t},
		output:  &output,
		errors:  &errors,
		version: "test-version",
		start: func(context.Context, io.Reader, io.Writer) error {
			t.Fatal("command arguments must not start the interactive TUI path")
			return nil
		},
		resume: func(context.Context, io.Reader, io.Writer) error { return nil },
		runCmd: testRunOutput,
		config: testConfigOutput,
	}

	err := runner.run(context.Background(), args)
	return output.String(), errors.String(), err
}

func runCLIWithConfigCommand(t *testing.T, args []string) (string, string, error) {
	t.Helper()

	var output bytes.Buffer
	var errors bytes.Buffer
	runner := cliRunner{
		input:   failReader{t: t},
		output:  &output,
		errors:  &errors,
		version: "test-version",
		start: func(context.Context, io.Reader, io.Writer) error {
			t.Fatal("config command must not start the interactive TUI path")
			return nil
		},
		resume: func(context.Context, io.Reader, io.Writer) error { return nil },
		runCmd: testRunOutput,
		config: app.ConfigCommandOutput,
	}

	err := runner.run(context.Background(), args)
	return output.String(), errors.String(), err
}

func runCLITestWithDebug(t *testing.T, args []string, debugOutput string) (string, string, error) {
	t.Helper()

	var output bytes.Buffer
	var errors bytes.Buffer
	runner := cliRunner{
		input:   failReader{t: t},
		output:  &output,
		errors:  &errors,
		version: "test-version",
		start: func(context.Context, io.Reader, io.Writer) error {
			t.Fatal("debug command must not start the interactive TUI path")
			return nil
		},
		resume: func(context.Context, io.Reader, io.Writer) error { return nil },
		runCmd: testRunOutput,
		debug: func(context.Context) (string, error) {
			return debugOutput, nil
		},
	}

	err := runner.run(context.Background(), args)
	return output.String(), errors.String(), err
}

func runCLIExitTest(t *testing.T, args []string) (string, string, int) {
	t.Helper()

	var output bytes.Buffer
	var errors bytes.Buffer
	runner := cliRunner{
		input:   failReader{t: t},
		output:  &output,
		errors:  &errors,
		version: "test-version",
		start: func(context.Context, io.Reader, io.Writer) error {
			t.Fatal("command arguments must not start the interactive TUI path")
			return nil
		},
		resume: func(context.Context, io.Reader, io.Writer) error { return nil },
		runCmd: testRunOutput,
		config: testConfigOutput,
	}

	done := make(chan error, 1)
	go func() {
		done <- runner.run(context.Background(), args)
	}()

	select {
	case err := <-done:
		if err == nil {
			return output.String(), errors.String(), 0
		}
		fmt.Fprintln(&errors, err)
		return output.String(), errors.String(), 1
	case <-time.After(time.Second):
		t.Fatalf("CLI args %v did not return without hanging", args)
		return "", "", 1
	}
}

func testRunOutput(_ context.Context, _ app.NonInteractiveRunRequest) (string, error) {
	return wantRunOutput, nil
}

func testConfigOutput(all bool) (string, error) {
	if all {
		return wantConfigAllOutput, nil
	}
	return wantConfigOutput, nil
}

func TestCLIRunnerBoundaryImports(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"main.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), filepath.FromSlash(path), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports for %s: %v", path, err)
		}

		for _, imported := range file.Imports {
			path := strings.Trim(imported.Path.Value, "\"")
			if strings.HasPrefix(path, "github.com/jgabor/aila/internal/") && path != "github.com/jgabor/aila/internal/app" {
				t.Fatalf("CLI runner imports forbidden internal dependency %q", path)
			}
		}
	}
}

func TestCLICommandParsingHasNoDirectIOReachability(t *testing.T) {
	t.Parallel()

	file, err := parser.ParseFile(token.NewFileSet(), filepath.FromSlash("main.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	guardedFunctions := map[string]bool{
		"run":                true,
		"parseCLI":           true,
		"isCLICommand":       true,
		"acceptsPositionals": true,
		"commandName":        true,
		"commandOutput":      true,
	}
	forbiddenSelectors := map[string]bool{
		"app.Run":             true,
		"os.Args":             true,
		"os.Stdin":            true,
		"os.Stdout":           true,
		"os.Stderr":           true,
		"os.Exit":             true,
		"os.Getenv":           true,
		"os.LookupEnv":        true,
		"os.ReadFile":         true,
		"os.WriteFile":        true,
		"os.Open":             true,
		"os.Create":           true,
		"os.Mkdir":            true,
		"os.MkdirAll":         true,
		"exec.Command":        true,
		"exec.CommandContext": true,
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !guardedFunctions[fn.Name.Name] {
			continue
		}

		ast.Inspect(fn.Body, func(node ast.Node) bool {
			if expr, ok := node.(*ast.SelectorExpr); ok {
				ident, ok := expr.X.(*ast.Ident)
				if ok && forbiddenSelectors[ident.Name+"."+expr.Sel.Name] {
					t.Fatalf("%s reaches forbidden IO/runtime selector %s.%s", fn.Name.Name, ident.Name, expr.Sel.Name)
				}
			}
			return true
		})
	}
}

type failReader struct {
	t *testing.T
}

func (r failReader) Read([]byte) (int, error) {
	r.t.Helper()
	r.t.Fatal("command-arg CLI read injected input")
	return 0, fmt.Errorf("unexpected read")
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}
