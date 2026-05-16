package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateMutationRequestsRejectUnsafePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, tc := range []struct {
		name string
		path string
		kind MutationErrorKind
	}{
		{name: "empty", path: "", kind: MutationErrorInvalidPath},
		{name: "traversal", path: "../outside.txt", kind: MutationErrorInvalidPath},
		{name: "home", path: "~/notes.txt", kind: MutationErrorReservedPath},
		{name: "xdg", path: "${XDG_CONFIG_HOME}/aila/config.toml", kind: MutationErrorReservedPath},
		{name: "directory", path: "docs/", kind: MutationErrorDirectoryLikePath},
		{name: "reserved", path: ".aila/project.toml", kind: MutationErrorReservedPath},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := ValidateWriteRequest(root, WriteRequest{Path: tc.path}); err.Kind != tc.kind {
				t.Fatalf("ValidateWriteRequest(%q) error = %+v, want %s", tc.path, err, tc.kind)
			}
			if _, err := ValidateEditRequest(root, EditRequest{Path: tc.path, OldText: "old"}); err.Kind != tc.kind {
				t.Fatalf("ValidateEditRequest(%q) error = %+v, want %s", tc.path, err, tc.kind)
			}
		})
	}
}

func TestValidateMutationRequestsNormalizeWorkspacePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	validated, err := ValidateWriteRequest(root, WriteRequest{Path: "docs/notes.txt", TargetVersion: MissingFileVersion, ExpectedEffect: "create notes"})
	if err.Kind != "" {
		t.Fatalf("ValidateWriteRequest error = %+v", err)
	}
	if validated.ToolName != WriteToolName || validated.WorkspaceRoot != root || validated.WorkspaceRelativePath != "docs/notes.txt" || validated.ResolvedPath != filepath.Join(root, "docs", "notes.txt") || validated.TargetVersion != MissingFileVersion || validated.ExpectedEffect != "create notes" {
		t.Fatalf("validated write = %+v", validated)
	}

	edit, err := ValidateEditRequest(root, EditRequest{Path: filepath.Join(root, "docs", "notes.txt"), TargetVersion: "sha256:old", OldText: "old", NewText: "new"})
	if err.Kind != "" {
		t.Fatalf("ValidateEditRequest error = %+v", err)
	}
	if !edit.RequestedPathWasAbsolute || edit.WorkspaceRelativePath != "docs/notes.txt" || edit.OldText != "old" || edit.NewText != "new" {
		t.Fatalf("validated edit = %+v", edit)
	}
}

func TestExecuteWriteCreatesAndOverwritesWithVersionCheck(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	request := validateWriteForTest(t, root, WriteRequest{Path: "docs/notes.txt", TargetVersion: MissingFileVersion, Content: "hello\n", ExpectedEffect: "create notes"})
	created := ExecuteWrite(context.Background(), request)
	if created.Error.Kind != "" || created.Status != "completed" || created.ToolName != WriteToolName || created.WorkspaceRelativePath != "docs/notes.txt" || created.PreviousExists || created.PreviousVersion != MissingFileVersion || !strings.HasPrefix(created.NewVersion, "sha256:") || created.BytesWritten != len("hello\n") || created.ExpectedEffect != "create notes" {
		t.Fatalf("created result = %+v", created)
	}
	if got := readMutationFile(t, filepath.Join(root, "docs", "notes.txt")); got != "hello\n" {
		t.Fatalf("created file = %q", got)
	}

	overwrite := validateWriteForTest(t, root, WriteRequest{Path: "docs/notes.txt", TargetVersion: created.NewVersion, Content: "updated\n", ExpectedEffect: "update notes"})
	updated := ExecuteWrite(context.Background(), overwrite)
	if updated.Error.Kind != "" || !updated.PreviousExists || updated.PreviousVersion != created.NewVersion || updated.NewVersion == created.NewVersion || updated.BytesWritten != len("updated\n") {
		t.Fatalf("updated result = %+v", updated)
	}
	if got := readMutationFile(t, filepath.Join(root, "docs", "notes.txt")); got != "updated\n" {
		t.Fatalf("updated file = %q", got)
	}
}

func TestExecuteWriteVersionMismatchDoesNotMutate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	writeTestFile(t, path, "original")
	request := validateWriteForTest(t, root, WriteRequest{Path: "notes.txt", TargetVersion: MissingFileVersion, Content: "changed"})
	result := ExecuteWrite(context.Background(), request)
	if result.Error.Kind != MutationErrorTargetVersionMismatch || result.Status != "failed" {
		t.Fatalf("result = %+v, want version mismatch", result)
	}
	if got := readMutationFile(t, path); got != "original" {
		t.Fatalf("file mutated on mismatch: %q", got)
	}
}

func TestExecuteEditReplacesExactlyOneMatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	writeTestFile(t, path, "alpha\nbeta\n")
	_, version, err := mutationFileVersion(path)
	if err.Kind != "" {
		t.Fatalf("version error = %+v", err)
	}
	request := validateEditForTest(t, root, EditRequest{Path: "notes.txt", TargetVersion: version, OldText: "beta", NewText: "gamma", ExpectedEffect: "replace beta"})
	result := ExecuteEdit(context.Background(), request)
	if result.Error.Kind != "" || result.Status != "completed" || result.ToolName != EditToolName || result.PreviousVersion != version || result.NewVersion == version || result.ReplacementCount != 1 || result.BytesWritten != len("alpha\ngamma\n") {
		t.Fatalf("edit result = %+v", result)
	}
	if got := readMutationFile(t, path); got != "alpha\ngamma\n" {
		t.Fatalf("edited file = %q", got)
	}
}

func TestExecuteEditMismatchDoesNotMutate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	writeTestFile(t, path, "alpha\nbeta\n")
	request := validateEditForTest(t, root, EditRequest{Path: "notes.txt", TargetVersion: "", OldText: "delta", NewText: "gamma"})
	result := ExecuteEdit(context.Background(), request)
	if result.Error.Kind != MutationErrorOldTextMismatch {
		t.Fatalf("edit result = %+v, want old text mismatch", result)
	}
	if got := readMutationFile(t, path); got != "alpha\nbeta\n" {
		t.Fatalf("file mutated on old-text mismatch: %q", got)
	}
}

func TestExecuteMutationRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeTestFile(t, outside, "outside")
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	request := validateWriteForTest(t, root, WriteRequest{Path: "link.txt", Content: "changed"})
	result := ExecuteWrite(context.Background(), request)
	if result.Error.Kind != MutationErrorSymlinkEscape {
		t.Fatalf("symlink result = %+v, want symlink escape", result)
	}
	if got := readMutationFile(t, outside); got != "outside" {
		t.Fatalf("outside target mutated: %q", got)
	}
}

func TestExecuteMutationHandlesCancellation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	request := validateWriteForTest(t, root, WriteRequest{Path: "notes.txt", Content: "changed"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := ExecuteWrite(ctx, request)
	if result.Error.Kind != MutationErrorCanceled {
		t.Fatalf("canceled result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("canceled write created file: %v", err)
	}
}

func TestMutationPackageDoesNotImportRuntimeWorkflowOrTUI(t *testing.T) {
	t.Parallel()

	for _, file := range []string{"mutation.go"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"internal/runtime", "internal/workflow", "internal/tui", "go-agent", "mcp", "plugin"} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("%s contains forbidden dependency marker %q", file, forbidden)
			}
		}
	}
}

func validateWriteForTest(t *testing.T, root string, request WriteRequest) ValidatedWriteRequest {
	t.Helper()
	validated, err := ValidateWriteRequest(root, request)
	if err.Kind != "" {
		t.Fatalf("ValidateWriteRequest() error = %+v", err)
	}
	return validated
}

func validateEditForTest(t *testing.T, root string, request EditRequest) ValidatedEditRequest {
	t.Helper()
	validated, err := ValidateEditRequest(root, request)
	if err.Kind != "" {
		t.Fatalf("ValidateEditRequest() error = %+v", err)
	}
	return validated
}

func readMutationFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
