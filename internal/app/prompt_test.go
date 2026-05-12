package app

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/tui"
)

func TestFakePromptHandlerReturnsDeterministicTypedResult(t *testing.T) {
	t.Parallel()

	handler := FakePromptHandler{}
	submission := PromptSubmission{Text: "  explain this repo  "}

	first := handler.Handle(submission)
	second := handler.Handle(submission)

	want := PromptResult{
		PromptText:    "explain this repo",
		AssistantText: "Fake Aila response: explain this repo",
	}
	if first != want {
		t.Fatalf("result = %+v, want %+v", first, want)
	}
	if second != first {
		t.Fatalf("handler is not deterministic: first=%+v second=%+v", first, second)
	}
}

func TestPromptBoundaryStaysMinimalAndIOFree(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "prompt.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse prompt boundary: %v", err)
	}

	imports := map[string]bool{}
	for _, spec := range parsed.Imports {
		imports[strings.Trim(spec.Path.Value, "\"")] = true
	}
	for _, forbidden := range []string{
		"context",
		"io",
		"os",
		"os/exec",
		"net/http",
		"github.com/jgabor/aila/internal/agent",
		"github.com/jgabor/aila/internal/capability",
		"github.com/jgabor/aila/internal/permission",
		"github.com/jgabor/aila/internal/runtime",
		"github.com/jgabor/aila/internal/state",
		"github.com/jgabor/aila/internal/tools",
		"github.com/jgabor/aila/internal/workflow",
	} {
		if imports[forbidden] {
			t.Fatalf("prompt boundary imports forbidden IO or future-scope package %q", forbidden)
		}
	}

	source, err := os.ReadFile("prompt.go")
	if err != nil {
		t.Fatalf("read prompt boundary: %v", err)
	}
	for _, forbidden := range []string{
		"type PromptHandler interface",
		"type Handler interface",
		"Router",
		"Provider",
		"Adapter",
		"Workflow",
		"Command",
		"Slash",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("prompt boundary contains future-scope abstraction %q", forbidden)
		}
	}
}

func TestPromptSubmitterRoutesThroughFakeHandler(t *testing.T) {
	t.Parallel()

	submit := newPromptSubmitter(FakePromptHandler{})
	result := submit("  /status  ")

	want := tui.TranscriptTurn{
		UserText:      "/status",
		AssistantText: "Fake Aila response: /status",
	}
	if result != want {
		t.Fatalf("submit result = %+v, want %+v", result, want)
	}
}
