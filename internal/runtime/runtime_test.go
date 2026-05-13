package runtime

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUpdateHandlesPromptDeterministically(t *testing.T) {
	t.Parallel()

	model := Model{Status: StatusIdle}
	firstModel, firstEffects := Update(model, PromptSubmitted{Text: "explain status"})
	secondModel, secondEffects := Update(model, PromptSubmitted{Text: "explain status"})

	if !reflect.DeepEqual(firstModel, secondModel) {
		t.Fatalf("Update is not deterministic for prompt model:\nfirst:  %#v\nsecond: %#v", firstModel, secondModel)
	}
	if !reflect.DeepEqual(firstEffects, secondEffects) {
		t.Fatalf("Update is not deterministic for prompt effects:\nfirst:  %#v\nsecond: %#v", firstEffects, secondEffects)
	}

	if firstModel.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", firstModel.Status, StatusActive)
	}
	if firstModel.NextOperation != 1 {
		t.Fatalf("NextOperation = %d, want 1", firstModel.NextOperation)
	}
	if got := firstModel.Transcript; !reflect.DeepEqual(got, []TranscriptEntry{{Kind: "prompt", Text: "explain status"}}) {
		t.Fatalf("Transcript = %#v", got)
	}

	if len(firstEffects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(firstEffects))
	}
	effect, ok := firstEffects[0].(FakePromptEffect)
	if !ok {
		t.Fatalf("effect type = %T, want FakePromptEffect", firstEffects[0])
	}
	if effect.Prompt != "explain status" {
		t.Fatalf("Prompt = %q", effect.Prompt)
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{
		ID:      "op-1",
		Kind:    OperationPrompt,
		Subject: "explain status",
		Source:  "user",
	})
}

func TestUpdateHandlesCommandDeterministically(t *testing.T) {
	t.Parallel()

	model := Model{Status: StatusIdle, NextOperation: 7}
	firstModel, firstEffects := Update(model, CommandSelected{Name: "status"})
	secondModel, secondEffects := Update(model, CommandSelected{Name: "status"})

	if !reflect.DeepEqual(firstModel, secondModel) {
		t.Fatalf("Update is not deterministic for command model:\nfirst:  %#v\nsecond: %#v", firstModel, secondModel)
	}
	if !reflect.DeepEqual(firstEffects, secondEffects) {
		t.Fatalf("Update is not deterministic for command effects:\nfirst:  %#v\nsecond: %#v", firstEffects, secondEffects)
	}
	if firstModel.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", firstModel.Status, StatusActive)
	}
	if firstModel.LastCommand != "status" {
		t.Fatalf("LastCommand = %q, want status", firstModel.LastCommand)
	}
	if len(firstEffects) != 1 {
		t.Fatalf("len(effects) = %d, want 1", len(firstEffects))
	}
	effect, ok := firstEffects[0].(FakeCommandEffect)
	if !ok {
		t.Fatalf("effect type = %T, want FakeCommandEffect", firstEffects[0])
	}
	if effect.Command != "status" {
		t.Fatalf("Command = %q", effect.Command)
	}
	assertOperationMetadata(t, effect.Metadata(), OperationMetadata{
		ID:      "op-8",
		Kind:    OperationCommand,
		Subject: "status",
		Source:  "user",
	})
}

func TestUpdateHandlesFakeResultMessages(t *testing.T) {
	t.Parallel()

	operation := OperationMetadata{ID: "op-3", Kind: OperationPrompt, Subject: "hello", Source: "user"}
	model := Model{
		Status:        StatusActive,
		NextOperation: 3,
		Transcript:    []TranscriptEntry{{Kind: "prompt", Text: "hello"}},
	}

	completed, effects := Update(model, FakeEffectCompleted{Operation: operation, Result: "fake answer"})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if completed.Status != StatusIdle {
		t.Fatalf("Status = %q, want %q", completed.Status, StatusIdle)
	}
	if completed.Result != "fake answer" {
		t.Fatalf("Result = %q, want fake answer", completed.Result)
	}
	if got := completed.Transcript[len(completed.Transcript)-1]; got != (TranscriptEntry{Kind: "result", Text: "fake answer"}) {
		t.Fatalf("last transcript = %#v", got)
	}

	failure := FailureMetadata{Code: "fake_failed", Message: "fake failure", Retryable: true}
	failed, effects := Update(model, FakeEffectFailed{Operation: operation, Failure: failure})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if failed.Status != StatusIdle {
		t.Fatalf("Status = %q, want %q", failed.Status, StatusIdle)
	}
	if failed.Result != "fake failure" {
		t.Fatalf("Result = %q, want fake failure", failed.Result)
	}
	if got := failed.Transcript[len(failed.Transcript)-1]; got != (TranscriptEntry{Kind: "failure", Text: "fake failure"}) {
		t.Fatalf("last transcript = %#v", got)
	}
}

func TestUpdateDoesNotMutateInputModel(t *testing.T) {
	t.Parallel()

	model := Model{
		Status:        StatusIdle,
		NextOperation: 2,
		Transcript:    []TranscriptEntry{{Kind: "result", Text: "previous"}},
	}
	original := Model{
		Status:        model.Status,
		NextOperation: model.NextOperation,
		Transcript:    append([]TranscriptEntry(nil), model.Transcript...),
	}

	updated, _ := Update(model, PromptSubmitted{Text: "next"})
	updated.Transcript[0].Text = "mutated copy"

	if !reflect.DeepEqual(model, original) {
		t.Fatalf("input model mutated:\ngot:  %#v\nwant: %#v", model, original)
	}
}

func TestFailureAndCancelMetadataAreInert(t *testing.T) {
	t.Parallel()

	metadata := OperationMetadata{
		ID:      "op-9",
		Kind:    OperationPrompt,
		Subject: "danger?",
		Source:  "user",
		Failure: FailureMetadata{Code: "bounded", Message: "bounded failure", Retryable: false},
		Cancel:  CancelMetadata{Requested: true, Reason: "user requested stop"},
	}

	model, effects := Update(Model{Status: StatusIdle}, FakeEffectFailed{
		Operation: metadata,
		Failure:   metadata.Failure,
	})
	if len(effects) != 0 {
		t.Fatalf("len(effects) = %d, want 0", len(effects))
	}
	if model.Status != StatusIdle {
		t.Fatalf("Status = %q, want %q", model.Status, StatusIdle)
	}
	if model.Result != metadata.Failure.Message {
		t.Fatalf("Result = %q, want %q", model.Result, metadata.Failure.Message)
	}
}

func TestRuntimeProductionFilesHaveNoForbiddenImportsOrTokens(t *testing.T) {
	t.Parallel()

	forbiddenImports := map[string]bool{
		"context":       true,
		"io":            true,
		"net/http":      true,
		"os":            true,
		"os/exec":       true,
		"path/filepath": true,
		"sync":          true,
	}
	forbiddenTokens := []string{
		"go ",
		"http.",
		"os.",
		"exec.",
		"Open(",
		"ReadFile(",
		"WriteFile(",
		"Mkdir",
		"Remove(",
		"Chdir(",
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbiddenTokens {
			if strings.Contains(string(content), token) {
				t.Fatalf("%s contains forbidden token %q", file, token)
			}
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), file, content, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			path := strings.Trim(imported.Path.Value, "\"")
			if forbiddenImports[path] {
				t.Fatalf("%s imports forbidden package %q", file, path)
			}
		}
	}
}

func assertOperationMetadata(t *testing.T, got OperationMetadata, want OperationMetadata) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata = %#v, want %#v", got, want)
	}
}
