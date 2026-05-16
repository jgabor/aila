package tools

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidateFindRequestAppliesTypedDefaults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	request := FindRequest{Pattern: "**/*.go", Source: SearchSourceMetadata{Caller: "model", RequestID: "find-1", Description: "discover go files"}}
	validated, err := ValidateFindRequest(root, request)
	if err.Kind != "" {
		t.Fatalf("ValidateFindRequest() error = %#v", err)
	}

	if validated.ToolName != FindToolName || validated.EffectivePattern != "**/*.go" {
		t.Fatalf("validated find identity = %#v", validated)
	}
	if validated.EffectiveMaxResults != DefaultSearchMaxResults || validated.EffectiveMaxPreviewBytes != DefaultSearchMaxPreviewBytes {
		t.Fatalf("validated find bounds = %#v", validated)
	}
	if validated.Source != request.Source {
		t.Fatalf("source = %#v, want %#v", validated.Source, request.Source)
	}
}

func TestValidateGrepRequestAppliesTypedDefaults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	request := GrepRequest{Query: "TODO", IncludePattern: "**/*.go", Source: SearchSourceMetadata{Caller: "model", RequestID: "grep-1"}}
	validated, err := ValidateGrepRequest(root, request)
	if err.Kind != "" {
		t.Fatalf("ValidateGrepRequest() error = %#v", err)
	}

	if validated.ToolName != GrepToolName || validated.EffectiveQuery != "TODO" || validated.EffectiveIncludePattern != "**/*.go" {
		t.Fatalf("validated grep identity = %#v", validated)
	}
	if validated.EffectiveMaxResults != DefaultSearchMaxResults || validated.EffectiveMaxPreviewBytes != DefaultSearchMaxPreviewBytes {
		t.Fatalf("validated grep bounds = %#v", validated)
	}
	if validated.Source != request.Source {
		t.Fatalf("source = %#v, want %#v", validated.Source, request.Source)
	}
}

func TestValidateSearchRequestsRejectUnsafeInputs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cases := []struct {
		name   string
		isFind bool
		find   FindRequest
		grep   GrepRequest
		kind   SearchErrorKind
	}{
		{name: "empty find pattern", isFind: true, kind: SearchErrorInvalidPattern},
		{name: "find traversal", isFind: true, find: FindRequest{Pattern: "../*.go"}, kind: SearchErrorInvalidPath},
		{name: "find absolute", isFind: true, find: FindRequest{Pattern: filepath.Join(t.TempDir(), "*.go")}, kind: SearchErrorOutsideWorkspace},
		{name: "find home", isFind: true, find: FindRequest{Pattern: "~/secret/*"}, kind: SearchErrorReservedPath},
		{name: "find xdg", isFind: true, find: FindRequest{Pattern: "$XDG_CONFIG_HOME/*"}, kind: SearchErrorReservedPath},
		{name: "find agentera", isFind: true, find: FindRequest{Pattern: ".agentera/*"}, kind: SearchErrorReservedPath},
		{name: "find aila", isFind: true, find: FindRequest{Pattern: ".aila/*"}, kind: SearchErrorReservedPath},
		{name: "find malformed", isFind: true, find: FindRequest{Pattern: "["}, kind: SearchErrorInvalidPattern},
		{name: "find negative max", isFind: true, find: FindRequest{Pattern: "*.go", MaxResults: -1}, kind: SearchErrorInvalidRange},
		{name: "grep empty query", grep: GrepRequest{IncludePattern: "*.go"}, kind: SearchErrorInvalidQuery},
		{name: "grep malformed regex", grep: GrepRequest{Query: "[", Regex: true}, kind: SearchErrorInvalidQuery},
		{name: "grep traversal include", grep: GrepRequest{Query: "x", IncludePattern: "../*.go"}, kind: SearchErrorInvalidPath},
		{name: "grep malformed include", grep: GrepRequest{Query: "x", IncludePattern: "["}, kind: SearchErrorInvalidPattern},
		{name: "grep negative preview", grep: GrepRequest{Query: "x", MaxPreviewBytes: -1}, kind: SearchErrorInvalidRange},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var err SearchError
			if tc.isFind {
				_, err = ValidateFindRequest(root, tc.find)
			} else {
				_, err = ValidateGrepRequest(root, tc.grep)
			}
			if err.Kind != tc.kind {
				t.Fatalf("error kind = %q, want %q (message %q)", err.Kind, tc.kind, err.Message)
			}
			if err.Message == "" || len(err.Message) > maxSearchErrorMessageBytes+3 {
				t.Fatalf("message = %q, want bounded non-empty", err.Message)
			}
		})
	}
}

func TestSearchResultShapesSuccessAndFailure(t *testing.T) {
	t.Parallel()

	matches := []SearchMatch{{Path: "b.txt"}, {Path: "a.txt", LineNumber: 2, PreviewText: strings.Repeat("x", 20)}}
	result := NewSearchSuccess(GrepToolName, "", "x", false, "*.txt", "/workspace", SearchSourceMetadata{Caller: "test"}, matches, 1, 8, 2, 1)
	if result.ToolName != GrepToolName || result.Query != "x" || result.IncludePattern != "*.txt" {
		t.Fatalf("result identity = %#v", result)
	}
	if result.Error.Kind != SearchErrorNone || !result.Truncation.ResultLimitHit || !result.Truncation.PreviewTruncated || result.Truncation.OmittedResults != 2 || result.Truncation.OmittedFiles != 1 {
		t.Fatalf("result truncation/error = %#v %#v", result.Truncation, result.Error)
	}
	if result.Matches[1].PreviewText != "xxxxxxxx" {
		t.Fatalf("bounded preview = %q", result.Matches[1].PreviewText)
	}
	if !strings.Contains(result.Truncation.TruncationMarkers, "result_limit_hit") || !strings.Contains(result.Truncation.TruncationMarkers, "files_omitted") {
		t.Fatalf("markers = %q", result.Truncation.TruncationMarkers)
	}

	failure := NewSearchFailure(FindToolName, "*.go", "", false, "", "", SearchSourceMetadata{}, SearchError{Kind: SearchErrorExecution, Message: strings.Repeat("x", 500)})
	if failure.Error.Kind != SearchErrorExecution || len(failure.Error.Message) > maxSearchErrorMessageBytes+3 || len(failure.Matches) != 0 {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestExecuteFindReturnsSortedBoundedMatches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "b", "two.go"), "package b\n")
	writeTestFile(t, filepath.Join(root, "a", "one.go"), "package a\n")
	writeTestFile(t, filepath.Join(root, "a", "skip.txt"), "skip\n")
	writeTestFile(t, filepath.Join(root, ".agentera", "hidden.go"), "package hidden\n")
	request, err := ValidateFindRequest(root, FindRequest{Pattern: "**/*.go", MaxResults: 1})
	if err.Kind != "" {
		t.Fatalf("ValidateFindRequest() error = %#v", err)
	}

	result := ExecuteFind(context.Background(), request)
	if result.Error.Kind != SearchErrorNone {
		t.Fatalf("ExecuteFind() error = %#v", result.Error)
	}
	if got, want := result.Matches, []SearchMatch{{Path: "a/one.go"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("matches = %#v, want %#v", got, want)
	}
	if !result.Truncation.ResultLimitHit || result.Truncation.OmittedResults != 1 {
		t.Fatalf("truncation = %#v, want one omitted result", result.Truncation)
	}
}

func TestExecuteGrepReturnsLineMatchesAndEmptySuccess(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "src", "app.go"), "alpha\nneedle here\n")
	writeTestFile(t, filepath.Join(root, "src", "other.go"), "needle second\n")
	writeTestFile(t, filepath.Join(root, "README.md"), "needle ignored by include\n")
	request, err := ValidateGrepRequest(root, GrepRequest{Query: "needle", IncludePattern: "**/*.go", MaxResults: 5})
	if err.Kind != "" {
		t.Fatalf("ValidateGrepRequest() error = %#v", err)
	}

	result := ExecuteGrep(context.Background(), request)
	if result.Error.Kind != SearchErrorNone {
		t.Fatalf("ExecuteGrep() error = %#v", result.Error)
	}
	want := []SearchMatch{{Path: "src/app.go", LineNumber: 2, PreviewText: "needle here"}, {Path: "src/other.go", LineNumber: 1, PreviewText: "needle second"}}
	if !reflect.DeepEqual(result.Matches, want) {
		t.Fatalf("matches = %#v, want %#v", result.Matches, want)
	}

	emptyRequest, err := ValidateGrepRequest(root, GrepRequest{Query: "missing", IncludePattern: "**/*.go"})
	if err.Kind != "" {
		t.Fatalf("ValidateGrepRequest(empty) error = %#v", err)
	}
	empty := ExecuteGrep(context.Background(), emptyRequest)
	if empty.Error.Kind != SearchErrorNone || len(empty.Matches) != 0 {
		t.Fatalf("empty grep = %#v, want success without matches", empty)
	}
}

func TestExecuteGrepHandlesRegexBinaryOversizedAndLimitMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a.txt"), "abc-123\nabc-456\n")
	writeTestFile(t, filepath.Join(root, "binary.txt"), "abc\x00hidden\n")
	writeTestFile(t, filepath.Join(root, "large.txt"), strings.Repeat("x", maxSearchLineBytes+1)+"abc\n")
	request, err := ValidateGrepRequest(root, GrepRequest{Query: `abc-\d+`, Regex: true, IncludePattern: "*.txt", MaxResults: 1, MaxPreviewBytes: 5})
	if err.Kind != "" {
		t.Fatalf("ValidateGrepRequest() error = %#v", err)
	}

	result := ExecuteGrep(context.Background(), request)
	if result.Error.Kind != SearchErrorNone {
		t.Fatalf("ExecuteGrep() error = %#v", result.Error)
	}
	if got := result.Matches; len(got) != 1 || got[0].Path != "a.txt" || got[0].LineNumber != 1 || got[0].PreviewText != "abc-1" {
		t.Fatalf("matches = %#v, want one bounded regex match", got)
	}
	if result.Truncation.OmittedResults != 1 || result.Truncation.OmittedFiles != 1 || !result.Truncation.PreviewTruncated {
		t.Fatalf("truncation = %#v, want omitted result, skipped files, and preview truncation", result.Truncation)
	}
}

func TestExecuteSearchRejectsCancellationAndDoesNotCreateDurableState(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	writeTestFile(t, filepath.Join(root, "notes.txt"), "needle\n")
	for _, dir := range []string{".git", ".agentera", "config"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotPaths(t, []string{filepath.Join(root, ".aila"), filepath.Join(root, ".git"), filepath.Join(root, ".agentera"), filepath.Join(root, "config"), home, filepath.Join(home, ".config")})

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	findRequest, findErr := ValidateFindRequest(root, FindRequest{Pattern: "*.txt"})
	if findErr.Kind != "" {
		t.Fatalf("ValidateFindRequest() error = %#v", findErr)
	}
	if got := ExecuteFind(canceled, findRequest).Error.Kind; got != SearchErrorCanceled {
		t.Fatalf("canceled find kind = %q", got)
	}
	grepRequest, grepErr := ValidateGrepRequest(root, GrepRequest{Query: "needle"})
	if grepErr.Kind != "" {
		t.Fatalf("ValidateGrepRequest() error = %#v", grepErr)
	}
	if got := ExecuteGrep(canceled, grepRequest).Error.Kind; got != SearchErrorCanceled {
		t.Fatalf("canceled grep kind = %q", got)
	}

	for i := 0; i < 3; i++ {
		if result := ExecuteFind(context.Background(), findRequest); result.Error.Kind != SearchErrorNone {
			t.Fatalf("ExecuteFind(%d) error = %#v", i, result.Error)
		}
		if result := ExecuteGrep(context.Background(), grepRequest); result.Error.Kind != SearchErrorNone {
			t.Fatalf("ExecuteGrep(%d) error = %#v", i, result.Error)
		}
	}
	after := snapshotPaths(t, []string{filepath.Join(root, ".aila"), filepath.Join(root, ".git"), filepath.Join(root, ".agentera"), filepath.Join(root, "config"), home, filepath.Join(home, ".config")})
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("durable state changed\nbefore:\n%s\nafter:\n%s", strings.Join(before, "\n"), strings.Join(after, "\n"))
	}
}

func TestSearchExecutorBoundaryKeepsIOOutOfRuntimeAndTUI(t *testing.T) {
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
			for _, forbidden := range []string{"ExecuteFind", "ExecuteGrep", "ValidateFindRequest", "ValidateGrepRequest", "internal/tools", "WalkDir", "os.Open", "os.ReadFile", "regexp.Compile"} {
				if strings.Contains(string(source), forbidden) {
					t.Fatalf("%s contains search executor or filesystem IO marker %q", file, forbidden)
				}
			}
		}
	}
}
