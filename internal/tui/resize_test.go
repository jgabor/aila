package tui

import (
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jgabor/aila/internal/policy"
)

func TestWindowSizeMsgUpdatesOnlyLayoutState(t *testing.T) {
	t.Parallel()

	var submitted []string
	var routed []policy.CommandRecommendation
	model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
		submitted = append(submitted, text)
		return TranscriptTurn{UserText: text, AssistantText: "Fake Aila response: " + text}
	}, func(recommendation policy.CommandRecommendation, state ViewState) ViewState {
		routed = append(routed, recommendation)
		return state
	})

	updated := updateNoCommand(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("explain this repo")})
	updated = updateNoCommand(t, updated, tea.KeyMsg{Type: tea.KeyEnter})
	updated = updateNoCommand(t, updated, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/status")})
	updated = updateNoCommand(t, updated, tea.KeyMsg{Type: tea.KeyEnter})
	updated = updateNoCommand(t, updated, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("draft")})

	beforeState := updated.state
	beforeSubmitted := append([]string(nil), submitted...)
	beforeRouted := append([]policy.CommandRecommendation(nil), routed...)

	afterModel, cmd := updated.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	if cmd != nil {
		t.Fatal("resize must not emit a Bubble Tea command")
	}
	after := afterModel.(Model)

	if !reflect.DeepEqual(after.state, beforeState) {
		t.Fatalf("resize mutated active content:\nbefore=%+v\nafter=%+v", beforeState, after.state)
	}
	if !reflect.DeepEqual(submitted, beforeSubmitted) {
		t.Fatalf("resize submitted prompts: before=%v after=%v", beforeSubmitted, submitted)
	}
	if !reflect.DeepEqual(routed, beforeRouted) {
		t.Fatalf("resize routed commands: before=%+v after=%+v", beforeRouted, routed)
	}
	if after.Layout() != (LayoutState{Size: Size{Width: 160, Height: 45}, Class: LayoutDesktop, RightRailVisible: true}) {
		t.Fatalf("layout after resize = %+v", after.Layout())
	}
}

func TestLayoutForFixedM6SizesIsDeterministic(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		size  Size
		class LayoutClass
		right bool
	}{
		{name: "80x24", size: Size{Width: 80, Height: 24}, class: LayoutCompact},
		{name: "100x30", size: Size{Width: 100, Height: 30}, class: LayoutStandard},
		{name: "120x32", size: Size{Width: 120, Height: 32}, class: LayoutSpacious},
		{name: "160x45", size: Size{Width: 160, Height: 45}, class: LayoutDesktop, right: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			model := NewModelWithSize(Size{Width: 1, Height: 1})
			updated, cmd := model.Update(tea.WindowSizeMsg{Width: tc.size.Width, Height: tc.size.Height})
			if cmd != nil {
				t.Fatal("resize must not emit a Bubble Tea command")
			}

			got := updated.(Model).Layout()
			want := LayoutState{Size: tc.size, Class: tc.class, RightRailVisible: tc.right}
			if got != want {
				t.Fatalf("layout = %+v, want %+v", got, want)
			}
		})
	}
}

func TestResizePathStaysPresentationOnlyAndIOFree(t *testing.T) {
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
				"net/http",
				"github.com/jgabor/aila/internal/app",
				"github.com/jgabor/aila/internal/agent",
				"github.com/jgabor/aila/internal/capability",
				"github.com/jgabor/aila/internal/config",
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
			for _, forbidden := range []string{
				"os.", "exec.", "http.", "net.Dial", ".aila", "git ", "Provider", "Permission", "StateStore", "Tool", "Config",
			} {
				if strings.Contains(string(source), forbidden) {
					t.Fatalf("TUI %s resize boundary contains IO or deferred ownership marker %q", file, forbidden)
				}
			}
		})
	}
}

func updateNoCommand(t *testing.T, model Model, msg tea.Msg) Model {
	t.Helper()

	updated, cmd := model.Update(msg)
	if cmd != nil {
		t.Fatalf("%T emitted unexpected Bubble Tea command", msg)
	}
	return updated.(Model)
}
