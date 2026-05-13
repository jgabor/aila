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
)

const wantRunStub = "aila test-version\n" +
	"command: run\n" +
	"status: deferred-run stub\n" +
	"accepted: run [prompt...] [--model MODEL]\n" +
	"deferred: prompt execution, stdin review, model turns, tool execution, workflow transitions\n"

const wantContinueStub = "aila test-version\n" +
	"command: continue\n" +
	"status: deferred-continuation stub\n" +
	"accepted: continue | --continue | -c\n" +
	"deferred: session discovery, state lookup, persistence IO, continuation execution\n"

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
	"opencode-go deepseek-v4-flash device_code utility available -\n" +
	"opencode-go deepseek-v4-pro device_code reasoning available -\n" +
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
	"M7 accepted shape:\n" +
	"  aila run [prompt...] [--model MODEL]\n" +
	"  aila continue | aila --continue | aila -c\n" +
	"  aila config [--all]\n" +
	"  aila models [filter...]\n" +
	"  aila help\n" +
	"  aila --version | aila -V\n" +
	"Deferred in M7: prompt execution, stdin review, session discovery, config IO, XDG/env reads, credentials, model turns, tools, workflow transitions, persistence.\n"

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
	if !strings.Contains(output.String(), "status: deferred-run stub") {
		t.Fatalf("command output did not stay within M7 boundary: %q", output.String())
	}
	if errors.Len() != 0 {
		t.Fatalf("command-arg CLI wrote unexpected stderr: %q", errors.String())
	}
}

func TestCLIRunnerRecognizesM7Commands(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"run":      "status: deferred-run stub",
		"continue": "status: deferred-continuation stub",
		"config":   "deferred: interactive config UI",
		"models":   "status: fake diagnostics",
		"help":     "M7 accepted shape:",
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

func TestCLIRunnerAcceptsGlobalFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "long model", args: []string{"--model", "openai/gpt", "run"}, want: wantRunStub},
		{name: "short model", args: []string{"-m", "openai/gpt", "run"}, want: wantRunStub},
		{name: "equals model", args: []string{"--model=openai/gpt", "run"}, want: wantRunStub},
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

func TestM7CLIAcceptedShapesExitWithExpectedStreams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "run", args: []string{"run"}, wantStdout: wantRunStub},
		{name: "run global model flag", args: []string{"--model", "openai/gpt", "run"}, wantStdout: wantRunStub},
		{name: "run short model flag", args: []string{"-m", "openai/gpt", "run"}, wantStdout: wantRunStub},
		{name: "run equals model flag", args: []string{"--model=openai/gpt", "run"}, wantStdout: wantRunStub},
		{name: "run prompt model", args: []string{"run", "write", "tests", "--model", "openai/gpt"}, wantStdout: wantRunStub},
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
		{name: "unknown command", args: []string{"status"}, want: "unknown command \"status\"; valid M7 commands: run, continue, config, models, help"},
		{name: "unknown positional after help", args: []string{"help", "topic"}, want: "unknown command \"topic\"; valid M7 commands: run, continue, config, models, help"},
		{name: "unknown flag", args: []string{"run", "--dry-run"}, want: "unknown flag \"--dry-run\"; valid M7 flags: --model, -m, --continue, -c, --version, -V"},
		{name: "missing model value", args: []string{"--model"}, want: "missing value for --model; valid M7 flags: --model, -m, --continue, -c, --version, -V"},
		{name: "missing command", args: []string{"--model", "openai/gpt"}, want: "missing command; valid M7 commands: run, continue, config, models, help"},
		{name: "config all on models", args: []string{"models", "--all"}, want: "unsupported M7 flag \"--all\" for models; valid config shape: config [--all]"},
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

func TestM7CLIUnknownInputsReturnBoundedDiagnostics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown command", args: []string{"status"}, want: "unknown command \"status\"; valid M7 commands: run, continue, config, models, help"},
		{name: "unknown flag", args: []string{"run", "--dry-run"}, want: "unknown flag \"--dry-run\"; valid M7 flags: --model, -m, --continue, -c, --version, -V"},
		{name: "unknown short flag", args: []string{"run", "-x"}, want: "unknown flag \"-x\"; valid M7 flags: --model, -m, --continue, -c, --version, -V"},
		{name: "missing model value", args: []string{"--model"}, want: "missing value for --model; valid M7 flags: --model, -m, --continue, -c, --version, -V"},
		{name: "empty model value", args: []string{"--model="}, want: "missing value for --model; valid M7 flags: --model, -m, --continue, -c, --version, -V"},
		{name: "missing command", args: []string{"--model", "openai/gpt"}, want: "missing command; valid M7 commands: run, continue, config, models, help"},
		{name: "ambiguous commands", args: []string{"run", "config"}, want: "incompatible M7 commands \"run\" and \"config\"; valid M7 commands: run, continue, config, models, help"},
		{name: "ambiguous continuation", args: []string{"run", "--continue"}, want: "incompatible M7 continuation shape \"run\" with --continue; use continue or --continue, not run --continue"},
		{name: "command specific flag", args: []string{"models", "--all"}, want: "unsupported M7 flag \"--all\" for models; valid config shape: config [--all]"},
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
		{name: "run continue flag", args: []string{"run", "--continue"}, wantErr: "incompatible M7 continuation shape \"run\" with --continue; use continue or --continue, not run --continue"},
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

func TestCLIRunnerRunStubDefersPromptStdinAndExecution(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runCLITest(t, []string{"run", "write", "code"})
	if err != nil {
		t.Fatalf("run prompt stub: %v", err)
	}
	if stdout != wantRunStub {
		t.Fatalf("stdout mismatch: got %q want %q", stdout, wantRunStub)
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
		config: app.ConfigCommandOutput,
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

func TestM7CLIStubCommandPathHasNoIOReachability(t *testing.T) {
	t.Parallel()

	file, err := parser.ParseFile(token.NewFileSet(), filepath.FromSlash("main.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	guardedFunctions := map[string]bool{
		"run":                  true,
		"parseM7CLI":           true,
		"isM7Command":          true,
		"acceptsM7Positionals": true,
		"commandName":          true,
		"m7StubOutput":         true,
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
