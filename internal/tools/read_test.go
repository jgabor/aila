package tools

import (
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
