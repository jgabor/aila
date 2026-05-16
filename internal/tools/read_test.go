package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateReadRequestAppliesTypedDefaults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	request := ReadRequest{
		Path: "docs/notes.txt",
		Source: ReadSourceMetadata{
			Caller:      "model",
			RequestID:   "turn-1",
			Description: "inspect note",
		},
	}

	validated, err := ValidateReadRequest(root, request)
	if err.Kind != "" {
		t.Fatalf("ValidateReadRequest() error = %#v", err)
	}

	if validated.ToolName != ReadToolName {
		t.Fatalf("ToolName = %q, want %q", validated.ToolName, ReadToolName)
	}
	if validated.RequestedPath != request.Path {
		t.Fatalf("RequestedPath = %q, want %q", validated.RequestedPath, request.Path)
	}
	if validated.WorkspaceRelativePath != "docs/notes.txt" {
		t.Fatalf("WorkspaceRelativePath = %q", validated.WorkspaceRelativePath)
	}
	if validated.ResolvedPath != filepath.Join(root, "docs", "notes.txt") {
		t.Fatalf("ResolvedPath = %q", validated.ResolvedPath)
	}
	if validated.EffectiveStartLine != DefaultReadStartLine {
		t.Fatalf("EffectiveStartLine = %d", validated.EffectiveStartLine)
	}
	if validated.EffectiveLineLimit != DefaultReadLineLimit {
		t.Fatalf("EffectiveLineLimit = %d", validated.EffectiveLineLimit)
	}
	if validated.EffectiveMaxPreviewBytes != DefaultReadMaxPreviewBytes {
		t.Fatalf("EffectiveMaxPreviewBytes = %d", validated.EffectiveMaxPreviewBytes)
	}
	if validated.Source != request.Source {
		t.Fatalf("Source = %#v, want %#v", validated.Source, request.Source)
	}
}

func TestValidateReadRequestPreservesExplicitRangeAndAbsoluteWorkspacePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	absPath := filepath.Join(root, "src", "main.go")
	validated, err := ValidateReadRequest(root, ReadRequest{
		Path:            absPath,
		StartLine:       7,
		LineLimit:       9,
		MaxPreviewBytes: 128,
	})
	if err.Kind != "" {
		t.Fatalf("ValidateReadRequest() error = %#v", err)
	}

	if !validated.RequestedPathWasAbsolute {
		t.Fatal("RequestedPathWasAbsolute = false, want true")
	}
	if validated.WorkspaceRelativePath != "src/main.go" {
		t.Fatalf("WorkspaceRelativePath = %q", validated.WorkspaceRelativePath)
	}
	if validated.RequestedStartLine != 7 || validated.EffectiveStartLine != 7 {
		t.Fatalf("start lines = requested %d effective %d", validated.RequestedStartLine, validated.EffectiveStartLine)
	}
	if validated.RequestedLineLimit != 9 || validated.EffectiveLineLimit != 9 {
		t.Fatalf("line limits = requested %d effective %d", validated.RequestedLineLimit, validated.EffectiveLineLimit)
	}
	if validated.RequestedMaxPreviewBytes != 128 || validated.EffectiveMaxPreviewBytes != 128 {
		t.Fatalf("preview bytes = requested %d effective %d", validated.RequestedMaxPreviewBytes, validated.EffectiveMaxPreviewBytes)
	}
}

func TestValidateReadRequestRejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cases := []struct {
		name string
		path string
		kind ReadErrorKind
	}{
		{name: "empty", path: "", kind: ReadErrorInvalidPath},
		{name: "absolute outside workspace", path: filepath.Join(t.TempDir(), "outside.txt"), kind: ReadErrorOutsideWorkspace},
		{name: "traversal", path: "../outside.txt", kind: ReadErrorInvalidPath},
		{name: "cleanable traversal", path: "src/../main.go", kind: ReadErrorInvalidPath},
		{name: "nested traversal", path: "src/../../outside.txt", kind: ReadErrorInvalidPath},
		{name: "home shorthand", path: "~/secret.txt", kind: ReadErrorReservedPath},
		{name: "home env", path: "$HOME/secret.txt", kind: ReadErrorReservedPath},
		{name: "xdg env", path: "$XDG_CONFIG_HOME/app", kind: ReadErrorReservedPath},
		{name: "agentera", path: ".agentera/plan.yaml", kind: ReadErrorReservedPath},
		{name: "aila", path: ".aila/session.json", kind: ReadErrorReservedPath},
		{name: "directory dot", path: ".", kind: ReadErrorDirectoryLikePath},
		{name: "directory slash", path: "docs/", kind: ReadErrorDirectoryLikePath},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := ValidateReadRequest(root, ReadRequest{Path: tc.path})
			if err.Kind != tc.kind {
				t.Fatalf("error kind = %q, want %q (message %q)", err.Kind, tc.kind, err.Message)
			}
			if err.Message == "" {
				t.Fatal("error message is empty")
			}
			if len(err.Message) > maxReadErrorMessageBytes+3 {
				t.Fatalf("error message length = %d, want bounded", len(err.Message))
			}
			if strings.Contains(err.Message, root) || (tc.path != "" && strings.Contains(err.Message, tc.path)) {
				t.Fatalf("error message %q leaks path %q or root %q", err.Message, tc.path, root)
			}
		})
	}
}

func TestValidateReadRequestRejectsInvalidRanges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cases := []ReadRequest{
		{Path: "file.txt", StartLine: -1},
		{Path: "file.txt", LineLimit: -1},
		{Path: "file.txt", MaxPreviewBytes: -1},
	}

	for _, request := range cases {
		_, err := ValidateReadRequest(root, request)
		if err.Kind != ReadErrorInvalidRange {
			t.Fatalf("ValidateReadRequest(%#v) kind = %q, want %q", request, err.Kind, ReadErrorInvalidRange)
		}
	}
}

func TestReadResultShapesSuccessAndFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	validated, err := ValidateReadRequest(root, ReadRequest{
		Path:            "src/app.go",
		StartLine:       3,
		LineLimit:       2,
		MaxPreviewBytes: 10,
		Source:          ReadSourceMetadata{Caller: "model", RequestID: "turn-2"},
	})
	if err.Kind != "" {
		t.Fatalf("ValidateReadRequest() error = %#v", err)
	}

	success := NewReadSuccess(validated, "line 3\nline 4\nline 5", 4, true)
	if success.ToolName != ReadToolName {
		t.Fatalf("ToolName = %q", success.ToolName)
	}
	if success.WorkspaceRelativePath != "src/app.go" {
		t.Fatalf("WorkspaceRelativePath = %q", success.WorkspaceRelativePath)
	}
	if success.ResolvedPath != filepath.Join(root, "src", "app.go") || !success.ResolvedPathAvailable {
		t.Fatalf("resolved provenance = %q available %v", success.ResolvedPath, success.ResolvedPathAvailable)
	}
	if success.RequestedRange != (ReadLineRange{StartLine: 3, Limit: 2}) {
		t.Fatalf("RequestedRange = %#v", success.RequestedRange)
	}
	if success.EffectiveRange != (ReadLineRange{StartLine: 3, EndLine: 4, Limit: 2}) {
		t.Fatalf("EffectiveRange = %#v", success.EffectiveRange)
	}
	if success.PreviewText != "line 3\nlin" {
		t.Fatalf("PreviewText = %q", success.PreviewText)
	}
	if !success.Truncation.PreviewTruncated || !success.Truncation.LineLimitHit || success.Truncation.Marker != "preview_truncated,line_limit_hit" {
		t.Fatalf("Truncation = %#v", success.Truncation)
	}
	if success.Error.Kind != ReadErrorNone {
		t.Fatalf("Error.Kind = %q", success.Error.Kind)
	}
	if success.Source != validated.Source {
		t.Fatalf("Source = %#v, want %#v", success.Source, validated.Source)
	}

	failure := NewReadFailure(validated, ReadError{Kind: ReadErrorExecution, Message: strings.Repeat("x", 500)})
	if failure.Error.Kind != ReadErrorExecution {
		t.Fatalf("failure kind = %q", failure.Error.Kind)
	}
	if len(failure.Error.Message) > maxReadErrorMessageBytes+3 {
		t.Fatalf("failure message length = %d, want bounded", len(failure.Error.Message))
	}
	if failure.PreviewText != "" {
		t.Fatalf("failure preview = %q, want empty", failure.PreviewText)
	}
}

func TestReadContractBoundaryExcludesFutureBehavior(t *testing.T) {
	t.Parallel()

	if ReadToolName != "read" {
		t.Fatalf("ReadToolName = %q", ReadToolName)
	}

	request := ReadRequest{Path: "README.md"}
	validated, err := ValidateReadRequest(t.TempDir(), request)
	if err.Kind != "" {
		t.Fatalf("ValidateReadRequest() error = %#v", err)
	}
	if validated.ToolName != "read" {
		t.Fatalf("validated tool = %q", validated.ToolName)
	}
}

func TestExecuteReadReturnsLineNumberedBoundedRange(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "notes.txt"), "alpha\nbeta\ngamma\ndelta\n")
	request := validateReadForTest(t, root, ReadRequest{Path: "notes.txt", StartLine: 2, LineLimit: 2, MaxPreviewBytes: 1024})

	result := ExecuteRead(context.Background(), request)
	if result.Error.Kind != ReadErrorNone {
		t.Fatalf("ExecuteRead() error = %#v", result.Error)
	}
	if result.PreviewText != "2: beta\n3: gamma\n" {
		t.Fatalf("PreviewText = %q", result.PreviewText)
	}
	if result.EffectiveRange != (ReadLineRange{StartLine: 2, EndLine: 3, Limit: 2}) {
		t.Fatalf("EffectiveRange = %#v", result.EffectiveRange)
	}
	if !result.Truncation.LineLimitHit || result.Truncation.PreviewTruncated {
		t.Fatalf("Truncation = %#v", result.Truncation)
	}
	if result.ResolvedPath != filepath.Join(root, "notes.txt") {
		t.Fatalf("ResolvedPath = %q", result.ResolvedPath)
	}
}

func TestExecuteReadReturnsTypedFailures(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "binary.bin"), "ok\x00no")
	writeTestFile(t, filepath.Join(root, "invalid-utf8.txt"), "ok\xffno")
	writeTestFile(t, filepath.Join(root, "locked.txt"), "secret\n")
	if err := os.Chmod(filepath.Join(root, "locked.txt"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "locked.txt"), 0o644) })
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(outside, "escape.txt"), "escape\n")
	symlinkPath := filepath.Join(root, "escape.txt")
	symlinkSupported := true
	if err := os.Symlink(filepath.Join(outside, "escape.txt"), symlinkPath); err != nil {
		if errors.Is(err, os.ErrPermission) {
			symlinkSupported = false
		} else {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name string
		path string
		kind ReadErrorKind
	}{
		{name: "missing", path: "missing.txt", kind: ReadErrorMissingFile},
		{name: "directory", path: "subdir", kind: ReadErrorDirectory},
		{name: "permission", path: "locked.txt", kind: ReadErrorPermission},
		{name: "binary", path: "binary.bin", kind: ReadErrorBinaryContent},
		{name: "invalid utf8", path: "invalid-utf8.txt", kind: ReadErrorBinaryContent},
	}
	if symlinkSupported {
		cases = append(cases, struct {
			name string
			path string
			kind ReadErrorKind
		}{name: "symlink escape", path: "escape.txt", kind: ReadErrorSymlinkEscape})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			request := validateReadForTest(t, root, ReadRequest{Path: tc.path})
			result := ExecuteRead(context.Background(), request)
			if result.Error.Kind != tc.kind {
				t.Fatalf("error kind = %q, want %q (message %q)", result.Error.Kind, tc.kind, result.Error.Message)
			}
			if result.PreviewText != "" {
				t.Fatalf("PreviewText = %q, want empty", result.PreviewText)
			}
			if result.Error.Message == "" || len(result.Error.Message) > maxReadErrorMessageBytes+3 {
				t.Fatalf("unbounded or empty message: %q", result.Error.Message)
			}
		})
	}
}

func TestExecuteReadHandlesOversizedFileAndLineWithTruncation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "large.txt"), strings.Repeat("a", 9000)+"\nsecond\nthird\n")
	request := validateReadForTest(t, root, ReadRequest{Path: "large.txt", StartLine: 1, LineLimit: 2, MaxPreviewBytes: 64})

	result := ExecuteRead(context.Background(), request)
	if result.Error.Kind != ReadErrorNone {
		t.Fatalf("ExecuteRead() error = %#v", result.Error)
	}
	if !strings.HasPrefix(result.PreviewText, "1: aaaaa") {
		t.Fatalf("PreviewText = %q", result.PreviewText)
	}
	if len(result.PreviewText) > 64 {
		t.Fatalf("PreviewText length = %d, want <= 64", len(result.PreviewText))
	}
	if result.EffectiveRange != (ReadLineRange{StartLine: 1, EndLine: 1, Limit: 2}) {
		t.Fatalf("EffectiveRange = %#v", result.EffectiveRange)
	}
	if !result.Truncation.PreviewTruncated || result.Truncation.LineLimitHit {
		t.Fatalf("Truncation = %#v", result.Truncation)
	}
	if !utf8Valid(result.PreviewText) {
		t.Fatalf("PreviewText is invalid UTF-8: %q", result.PreviewText)
	}
}

func TestExecuteReadRejectsOversizedSkippedContent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "large.txt"), strings.Repeat("skip\n", (maxReadScanBytes/5)+2))
	request := validateReadForTest(t, root, ReadRequest{Path: "large.txt", StartLine: maxReadScanBytes, LineLimit: 1, MaxPreviewBytes: 64})

	result := ExecuteRead(context.Background(), request)
	if result.Error.Kind != ReadErrorOversizedFile {
		t.Fatalf("error kind = %q, want %q", result.Error.Kind, ReadErrorOversizedFile)
	}
	if result.PreviewText != "" {
		t.Fatalf("PreviewText = %q, want empty", result.PreviewText)
	}
}

func TestExecuteReadRejectsInvalidEffectiveRangeAndCancellation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "notes.txt"), "alpha\n")
	request := validateReadForTest(t, root, ReadRequest{Path: "notes.txt"})

	invalid := request
	invalid.EffectiveLineLimit = 0
	invalidResult := ExecuteRead(context.Background(), invalid)
	if invalidResult.Error.Kind != ReadErrorInvalidRange {
		t.Fatalf("invalid range kind = %q", invalidResult.Error.Kind)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	canceledResult := ExecuteRead(canceled, request)
	if canceledResult.Error.Kind != ReadErrorCanceled {
		t.Fatalf("canceled kind = %q", canceledResult.Error.Kind)
	}
}

func TestExecuteReadDoesNotCreateDurableState(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	writeTestFile(t, filepath.Join(root, "notes.txt"), "alpha\nbeta\n")
	writeTestFile(t, filepath.Join(root, "unrelated.txt"), "keep\n")
	for _, dir := range []string{".git", ".agentera", "config"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotPaths(t, []string{
		filepath.Join(root, ".aila"),
		filepath.Join(root, ".git"),
		filepath.Join(root, ".agentera"),
		filepath.Join(root, "config"),
		filepath.Join(root, "unrelated.txt"),
		home,
		filepath.Join(home, ".config"),
	})
	request := validateReadForTest(t, root, ReadRequest{Path: "notes.txt"})

	for i := 0; i < 5; i++ {
		result := ExecuteRead(context.Background(), request)
		if result.Error.Kind != ReadErrorNone {
			t.Fatalf("ExecuteRead(%d) error = %#v", i, result.Error)
		}
	}

	after := snapshotPaths(t, []string{
		filepath.Join(root, ".aila"),
		filepath.Join(root, ".git"),
		filepath.Join(root, ".agentera"),
		filepath.Join(root, "config"),
		filepath.Join(root, "unrelated.txt"),
		home,
		filepath.Join(home, ".config"),
	})
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("durable state changed\nbefore:\n%s\nafter:\n%s", strings.Join(before, "\n"), strings.Join(after, "\n"))
	}
}

func TestReadExecutorBoundaryKeepsIOOutOfRuntimeAndTUI(t *testing.T) {
	t.Parallel()

	patterns := []string{"../runtime/*.go", "../tui/*.go"}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"ExecuteRead", "ValidateReadRequest", "internal/tools", "os.Open", "os.ReadFile", "EvalSymlinks"} {
				if strings.Contains(string(source), forbidden) {
					t.Fatalf("%s contains read executor or filesystem IO marker %q", file, forbidden)
				}
			}
		}
	}
}

func validateReadForTest(t *testing.T, root string, request ReadRequest) ValidatedReadRequest {
	t.Helper()

	validated, err := ValidateReadRequest(root, request)
	if err.Kind != "" {
		t.Fatalf("ValidateReadRequest() error = %#v", err)
	}
	return validated
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func snapshotPaths(t *testing.T, paths []string) []string {
	t.Helper()

	snapshots := make([]string, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			snapshots = append(snapshots, path+":missing")
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		entry := path + ":" + info.Mode().String()
		if !info.IsDir() {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			entry += ":" + string(content)
		}
		snapshots = append(snapshots, entry)
	}
	return snapshots
}

func utf8Valid(text string) bool {
	return strings.ToValidUTF8(text, "") == text
}
