package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateBashRequestAllowsOnlySafeInspectionCommands(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	cases := []struct {
		name   string
		argv   []string
		family string
	}{
		{name: "pwd", argv: []string{"pwd"}, family: "pwd"},
		{name: "ls", argv: []string{"ls", "-la", "."}, family: "ls"},
		{name: "git status", argv: []string{"git", "status", "--short", "--branch"}, family: "git status"},
		{name: "git diff", argv: []string{"git", "diff", "--stat", "--", "README.md"}, family: "git diff"},
		{name: "git ls-files", argv: []string{"git", "ls-files", "--others", "--exclude-standard"}, family: "git ls-files"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			validated, err := ValidateBashRequest(root, BashRequest{Argv: tc.argv, WorkingDir: ".", Source: BashSourceMetadata{Caller: "test", RequestID: tc.name}})
			if err.Kind != "" {
				t.Fatalf("ValidateBashRequest(%v) error = %+v", tc.argv, err)
			}
			if validated.ToolName != BashToolName || validated.CommandFamily != tc.family || validated.ExpectedEffect == "" || validated.WorkspaceRelativeWorkDir != "." {
				t.Fatalf("validated = %+v", validated)
			}
			if validated.EffectiveMaxOutputBytes != DefaultBashMaxOutputBytes || validated.EffectiveTimeoutMillis != DefaultBashTimeoutMillis {
				t.Fatalf("defaults = output %d timeout %d", validated.EffectiveMaxOutputBytes, validated.EffectiveTimeoutMillis)
			}
			if tc.family == "git diff" && !sameStrings(validated.EffectiveArgv[:3], []string{"git", "diff", "--no-ext-diff"}) {
				t.Fatalf("git diff effective argv = %v, want --no-ext-diff safety flag", validated.EffectiveArgv)
			}
		})
	}
}

func TestValidateBashRequestRejectsUnsafeCommands(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	cases := []struct {
		name string
		req  BashRequest
		kind BashErrorKind
	}{
		{name: "empty", req: BashRequest{}, kind: BashErrorInvalidCommand},
		{name: "shell string", req: BashRequest{Argv: []string{"git status"}}, kind: BashErrorUnsafeCommand},
		{name: "pipeline", req: BashRequest{Argv: []string{"git", "status", "|", "cat"}}, kind: BashErrorUnsafeCommand},
		{name: "redirection", req: BashRequest{Argv: []string{"git", "diff", ">", "out"}}, kind: BashErrorUnsafeCommand},
		{name: "env assignment", req: BashRequest{Argv: []string{"GIT_DIR=.git", "git", "status"}}, kind: BashErrorUnsafeCommand},
		{name: "home path", req: BashRequest{Argv: []string{"ls", "$HOME/.ssh"}}, kind: BashErrorUnsafeCommand},
		{name: "wildcard", req: BashRequest{Argv: []string{"ls", "*.go"}}, kind: BashErrorInvalidPath},
		{name: "reserved", req: BashRequest{Argv: []string{"ls", ".aila"}}, kind: BashErrorReservedPath},
		{name: "traversal", req: BashRequest{Argv: []string{"ls", "../outside"}}, kind: BashErrorInvalidPath},
		{name: "mutation git", req: BashRequest{Argv: []string{"git", "checkout", "main"}}, kind: BashErrorUnsafeCommand},
		{name: "network", req: BashRequest{Argv: []string{"curl", "https://example.test"}}, kind: BashErrorUnsafeCommand},
		{name: "bad workdir", req: BashRequest{Argv: []string{"pwd"}, WorkingDir: "../outside"}, kind: BashErrorInvalidPath},
		{name: "bad bounds", req: BashRequest{Argv: []string{"pwd"}, MaxOutputBytes: -1}, kind: BashErrorInvalidRange},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ValidateBashRequest(root, tc.req)
			if err.Kind != tc.kind {
				t.Fatalf("ValidateBashRequest(%v) error = %+v, want kind %s", tc.req.Argv, err, tc.kind)
			}
		})
	}
}

func TestExecuteBashRunsSafeInspectionWithoutMutation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "notes.txt"), "alpha\nbeta\n")
	before := snapshotDir(t, root)

	validated, err := ValidateBashRequest(root, BashRequest{Argv: []string{"ls", "-1"}, MaxOutputBytes: 20, Source: BashSourceMetadata{Caller: "test", RequestID: "ls-1"}})
	if err.Kind != "" {
		t.Fatalf("validate ls: %+v", err)
	}
	result := ExecuteBash(context.Background(), validated)
	if result.Error.Kind != BashErrorNone || result.Status != "completed" || result.ExitCode != 0 || result.CommandFamily != "ls" {
		t.Fatalf("ls result = %+v", result)
	}
	if !strings.Contains(result.Stdout.Text, "notes.txt") || result.Stdout.Bytes == 0 {
		t.Fatalf("stdout = %+v, want notes.txt", result.Stdout)
	}
	if after := snapshotDir(t, root); after != before {
		t.Fatalf("safe command mutated workspace: before=%q after=%q", before, after)
	}
	if _, err := os.Stat(filepath.Join(root, ".aila")); !os.IsNotExist(err) {
		t.Fatalf("safe command created .aila: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agentera")); !os.IsNotExist(err) {
		t.Fatalf("safe command created .agentera: %v", err)
	}
}

func TestExecuteBashSurfacesFailureAndTimeout(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	validated, err := ValidateBashRequest(root, BashRequest{Argv: []string{"git", "status", "--short"}, MaxOutputBytes: 80})
	if err.Kind != "" {
		t.Fatalf("validate git status: %+v", err)
	}
	failed := ExecuteBash(context.Background(), validated)
	if failed.Error.Kind != BashErrorExecution || failed.Status != "failed" || failed.ExitCode == 0 || failed.Stderr.Text == "" {
		t.Fatalf("git status outside repo result = %+v, want bounded failure", failed)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := ExecuteBash(ctx, validated)
	if canceled.Error.Kind != BashErrorCanceled || canceled.Status != "canceled" {
		t.Fatalf("canceled result = %+v", canceled)
	}

	timeoutReq := validated
	timeoutReq.EffectiveArgv = []string{"git", "status", "--short"}
	timeoutReq.EffectiveTimeoutMillis = 1
	_ = time.Now()
	timed := ExecuteBash(context.Background(), timeoutReq)
	if timed.Error.Kind != BashErrorNone && timed.Error.Kind != BashErrorTimeout && timed.Error.Kind != BashErrorExecution {
		t.Fatalf("unexpected tiny-timeout result = %+v", timed)
	}
}

func TestBashContractBoundaryNamesExcludedFutureScope(t *testing.T) {
	t.Parallel()
	text := readRepoFile(t, "bash.go")
	for _, forbidden := range []string{"FetchToolName", "EditTool", "WriteTool", "workflow.", "internal/agent", "mcp", "plugin"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("bash contract includes forbidden future scope %q", forbidden)
		}
	}
	if strings.Contains(text, "sh -c") || strings.Contains(text, "bash -c") {
		t.Fatalf("bash executor must not shell-evaluate commands")
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func snapshotDir(t *testing.T, root string) string {
	t.Helper()
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return strings.Join(paths, "\n")
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func sameStrings(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
