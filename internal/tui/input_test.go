package tui

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPromptTypingUpdatesLocalViewStateOnly(t *testing.T) {
	t.Parallel()

	var routed []string
	model := NewModelWithSizeAndPromptSubmit(Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
		routed = append(routed, text)
		return TranscriptTurn{}
	})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")})
	got := updated.(Model)

	if cmd != nil {
		t.Fatal("typed prompt input must not emit a Bubble Tea command")
	}
	if len(routed) != 0 {
		t.Fatalf("typed prompt input routed app messages: %v", routed)
	}
	if got.PromptInput() != "hi" {
		t.Fatalf("prompt input = %q, want hi", got.PromptInput())
	}
	if !strings.Contains(got.View(), "> hi") {
		t.Fatalf("view does not show local prompt input:\n%s", got.View())
	}
}

func TestPromptBackspaceUpdatesLocalViewStateOnly(t *testing.T) {
	t.Parallel()

	var routed []string
	model := NewModelWithSizeAndPromptSubmit(Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
		routed = append(routed, text)
		return TranscriptTurn{}
	})
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hey")})
	if cmd != nil {
		t.Fatal("typing setup must not emit a command")
	}

	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyBackspace})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("backspace must not emit a Bubble Tea command")
	}
	if len(routed) != 0 {
		t.Fatalf("backspace routed app messages: %v", routed)
	}
	if got.PromptInput() != "he" {
		t.Fatalf("prompt input after backspace = %q, want he", got.PromptInput())
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("ctrl+h backspace equivalent must not emit a Bubble Tea command")
	}
	if len(routed) != 0 {
		t.Fatalf("ctrl+h routed app messages: %v", routed)
	}
	if got.PromptInput() != "h" {
		t.Fatalf("prompt input after ctrl+h = %q, want h", got.PromptInput())
	}
	if !strings.Contains(got.View(), "> h") {
		t.Fatalf("view does not show edited prompt input:\n%s", got.View())
	}
}

func TestPromptEnterRoutesNonEmptyInputWithoutSlashInterpretation(t *testing.T) {
	t.Parallel()

	var routed []string
	model := NewModelWithSizeAndPromptSubmit(Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
		routed = append(routed, text)
		return TranscriptTurn{UserText: text, AssistantText: "Fake Aila response: " + text}
	})
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/status")})
	if cmd != nil {
		t.Fatal("typing setup must not emit a command")
	}

	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("enter submit must route synchronously without a Bubble Tea command")
	}
	if got.PromptInput() != "" {
		t.Fatalf("prompt input after submit = %q, want cleared", got.PromptInput())
	}
	if len(routed) != 1 || routed[0] != "/status" {
		t.Fatalf("routed prompts = %v, want [/status]", routed)
	}
	if !strings.Contains(got.View(), "  user: /status\n  assistant: Fake Aila response: /status") {
		t.Fatalf("view does not show submitted transcript in order:\n%s", got.View())
	}
}

func TestFakeResponseTranscriptRenderingIsStable(t *testing.T) {
	t.Parallel()

	render := func() string {
		state := IdleEmptyState()
		state.Transcript = []TranscriptTurn{{
			UserText:      "explain this repo",
			AssistantText: "Fake Aila response: explain this repo",
		}}
		return RenderPlain(state, Size{Width: 80, Height: 24})
	}

	first := render()
	second := render()
	if first != second {
		t.Fatalf("transcript render is not stable:\nfirst:\n%s\n\nsecond:\n%s", first, second)
	}
	assertOrdered(t, first, "  user: explain this repo", "  assistant: Fake Aila response: explain this repo")
	for _, forbidden := range []string{"2026-", "T16:", "timestamp", "time:"} {
		if strings.Contains(first, forbidden) {
			t.Fatalf("transcript render contains unstable time marker %q:\n%s", forbidden, first)
		}
	}
}

func assertOrdered(t *testing.T, value string, before string, after string) {
	t.Helper()

	beforeIndex := strings.Index(value, before)
	afterIndex := strings.Index(value, after)
	if beforeIndex < 0 || afterIndex < 0 || beforeIndex >= afterIndex {
		t.Fatalf("render does not contain %q before %q:\n%s", before, after, value)
	}
}

func TestPromptEnterOnEmptyInputDoesNothing(t *testing.T) {
	t.Parallel()

	var routed []string
	model := NewModelWithSizeAndPromptSubmit(Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
		routed = append(routed, text)
		return TranscriptTurn{UserText: text, AssistantText: "unexpected"}
	})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("empty enter must not emit a Bubble Tea command")
	}
	if len(routed) != 0 {
		t.Fatalf("empty enter routed app messages: %v", routed)
	}
	if got.PromptInput() != "" {
		t.Fatalf("prompt input after empty enter = %q, want empty", got.PromptInput())
	}
	if strings.Contains(got.View(), "Fake Aila response") {
		t.Fatalf("empty enter rendered a fake response:\n%s", got.View())
	}
}

func TestPromptInputUpdateStaysPresentationOnly(t *testing.T) {
	t.Parallel()

	for _, file := range []string{"model.go", "render.go"} {
		file := file
		t.Run(file, func(t *testing.T) {
			t.Parallel()

			fileSet := token.NewFileSet()
			parsed, err := parser.ParseFile(fileSet, file, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse TUI %s: %v", file, err)
			}

			imports := map[string]bool{}
			for _, spec := range parsed.Imports {
				imports[strings.Trim(spec.Path.Value, "\"")] = true
			}
			for _, forbidden := range []string{
				"os",
				"os/exec",
				"time",
				"net/http",
				"github.com/jgabor/aila/internal/app",
				"github.com/jgabor/aila/internal/agent",
				"github.com/jgabor/aila/internal/capability",
				"github.com/jgabor/aila/internal/permission",
				"github.com/jgabor/aila/internal/runtime",
				"github.com/jgabor/aila/internal/state",
				"github.com/jgabor/aila/internal/tools",
				"github.com/jgabor/aila/internal/workflow",
			} {
				if imports[forbidden] {
					t.Fatalf("TUI %s imports forbidden IO or ownership package %q", file, forbidden)
				}
			}

			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read TUI %s: %v", file, err)
			}
			for _, forbidden := range []string{"Slash", "CommandRouter", "Provider", "Adapter", "exec.Command", "git "} {
				if strings.Contains(string(source), forbidden) {
					t.Fatalf("TUI %s contains future-scope or IO behavior marker %q", file, forbidden)
				}
			}
		})
	}
}
