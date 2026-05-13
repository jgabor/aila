package tui

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jgabor/aila/internal/policy"
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

func TestModelAcceptsAppOwnedDisplayStateWithoutChangingInputBehavior(t *testing.T) {
	t.Parallel()

	state := IdleEmptyState()
	state.PrimaryModel = "test-primary"
	state.UtilityModel = "test-utility"
	state.Autonomy = "test-auto"
	model := NewModelWithStateSizePromptSubmitAndCommandRoute(state, Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
		return TranscriptTurn{UserText: text, AssistantText: "Fake Aila response: " + text}
	}, nil)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	if cmd != nil {
		t.Fatal("typing with app-owned display state must not emit a command")
	}
	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("ordinary prompt submit with app-owned display state must not emit a command")
	}
	if !containsAll(got.View(), []string{"Model test-primary", "Utility test-utility", "Auto test-auto", "user: hello", "assistant: Fake Aila response: hello"}) {
		t.Fatalf("view missing app-owned labels or prompt transcript:\n%s", got.View())
	}
}

func TestDefaultModelDoesNotOwnWorkflowPhase(t *testing.T) {
	t.Parallel()

	view := NewModelWithSize(Size{Width: 80, Height: 24}).View()
	if !strings.Contains(view, "Stage  | Model placeholder | Utility placeholder | Auto placeholder") {
		t.Fatalf("default TUI model should leave phase injection to the app:\n%s", view)
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

func TestPromptEnterRoutesRecognizedCommandThroughPolicyBoundary(t *testing.T) {
	t.Parallel()

	var routed []string
	var commands []policy.CommandRecommendation
	model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
		routed = append(routed, text)
		return TranscriptTurn{UserText: text, AssistantText: "Fake Aila response: " + text}
	}, func(recommendation policy.CommandRecommendation) {
		commands = append(commands, recommendation)
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
	if len(routed) != 0 {
		t.Fatalf("recognized command routed fake prompts: %v", routed)
	}
	if len(commands) != 1 || commands[0].Route != policy.CommandRouteStatus || commands[0].Kind != policy.CommandInputSlash {
		t.Fatalf("commands = %+v, want slash status recommendation", commands)
	}
	if strings.Contains(got.View(), "Fake Aila response") || strings.Contains(got.View(), "user: /status") {
		t.Fatalf("recognized command rendered fake prompt transcript:\n%s", got.View())
	}
}

func TestCommandShortcutsUseSamePolicyRoutesAsSlashCommands(t *testing.T) {
	t.Parallel()

	var routed []string
	var commands []policy.CommandRecommendation
	model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
		routed = append(routed, text)
		return TranscriptTurn{UserText: text, AssistantText: "unexpected"}
	}, func(recommendation policy.CommandRecommendation) {
		commands = append(commands, recommendation)
	})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if cmd != nil {
		t.Fatal("ctrl+x prefix must not emit a Bubble Tea command")
	}
	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd != nil {
		t.Fatal("ctrl+x s route must not emit a Bubble Tea command in Task 2")
	}
	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if cmd != nil {
		t.Fatal("second ctrl+x prefix must not emit a Bubble Tea command")
	}
	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("ctrl+x q route must emit Bubble Tea quit command")
	}
	got := updated.(Model)

	if len(routed) != 0 {
		t.Fatalf("shortcuts routed fake prompts: %v", routed)
	}
	if !got.Quitting() {
		t.Fatal("ctrl+x q command route should mark the model quitting")
	}
	if len(commands) != 2 {
		t.Fatalf("commands = %+v, want status and quit", commands)
	}
	if commands[0].Route != policy.CommandRouteStatus || commands[0].Kind != policy.CommandInputShortcut {
		t.Fatalf("first command = %+v, want shortcut status", commands[0])
	}
	if commands[1].Route != policy.CommandRouteQuit || commands[1].Kind != policy.CommandInputShortcut {
		t.Fatalf("second command = %+v, want shortcut quit", commands[1])
	}
}

func TestSlashAndShortcutParityAtPolicyAndTUIBoundaries(t *testing.T) {
	t.Parallel()

	statusSlash, ok := policy.RecommendSlashCommand("/status")
	if !ok {
		t.Fatal("/status did not match")
	}
	statusShortcut, ok := policy.RecommendShortcut("ctrl+x", "s")
	if !ok {
		t.Fatal("ctrl+x s did not match")
	}
	if statusSlash.Route != statusShortcut.Route {
		t.Fatalf("status policy route mismatch: slash=%+v shortcut=%+v", statusSlash, statusShortcut)
	}

	quitSlash, ok := policy.RecommendSlashCommand("/quit")
	if !ok {
		t.Fatal("/quit did not match")
	}
	quitShortcut, ok := policy.RecommendShortcut("ctrl+x", "q")
	if !ok {
		t.Fatal("ctrl+x q did not match")
	}
	if quitSlash.Route != quitShortcut.Route {
		t.Fatalf("quit policy route mismatch: slash=%+v shortcut=%+v", quitSlash, quitShortcut)
	}

	statusSlashModel, statusSlashCmd := routeSlashCommandForParity(t, "/status")
	statusShortcutModel, statusShortcutCmd := routeShortcutForParity(t, "s")
	if statusSlashCmd != nil || statusShortcutCmd != nil {
		t.Fatal("status parity routes should not emit Bubble Tea commands")
	}
	if statusSlashModel.state.CommandRoute != statusShortcutModel.state.CommandRoute || statusSlashModel.state.RouteSource != statusShortcutModel.state.RouteSource {
		t.Fatalf("status TUI route mismatch: slash=%+v shortcut=%+v", statusSlashModel.state, statusShortcutModel.state)
	}
	if statusSlashModel.state.SurfaceTitle != statusShortcutModel.state.SurfaceTitle || strings.Join(statusSlashModel.state.SurfaceLines, "\n") != strings.Join(statusShortcutModel.state.SurfaceLines, "\n") {
		t.Fatalf("status TUI surface mismatch:\nslash=%s %v\nshortcut=%s %v", statusSlashModel.state.SurfaceTitle, statusSlashModel.state.SurfaceLines, statusShortcutModel.state.SurfaceTitle, statusShortcutModel.state.SurfaceLines)
	}

	quitSlashModel, quitSlashCmd := routeSlashCommandForParity(t, "/quit")
	quitShortcutModel, quitShortcutCmd := routeShortcutForParity(t, "q")
	if quitSlashModel.state.CommandRoute != quitShortcutModel.state.CommandRoute || quitSlashModel.state.RouteSource != quitShortcutModel.state.RouteSource {
		t.Fatalf("quit TUI route mismatch: slash=%+v shortcut=%+v", quitSlashModel.state, quitShortcutModel.state)
	}
	if !quitSlashModel.Quitting() || !quitShortcutModel.Quitting() || quitSlashCmd == nil || quitShortcutCmd == nil {
		t.Fatal("/quit and ctrl+x q parity boundary must share command-route quit behavior")
	}
	if quitSlashModel.state.SurfaceTitle != "" || quitShortcutModel.state.SurfaceTitle != "" {
		t.Fatalf("quit parity should not render a command surface yet: slash=%q shortcut=%q", quitSlashModel.state.SurfaceTitle, quitShortcutModel.state.SurfaceTitle)
	}
}

func routeSlashCommandForParity(t *testing.T, input string) (Model, tea.Cmd) {
	t.Helper()

	model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 80, Height: 24}, nil, nil)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(input)})
	if cmd != nil {
		t.Fatalf("typing %s emitted a command", input)
	}
	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if input != "/quit" && cmd != nil {
		t.Fatalf("routing %s emitted a command", input)
	}
	return updated.(Model), cmd
}

func routeShortcutForParity(t *testing.T, key string) (Model, tea.Cmd) {
	t.Helper()

	model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 80, Height: 24}, nil, nil)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if cmd != nil {
		t.Fatal("ctrl+x prefix emitted a command")
	}
	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	if key != "q" && cmd != nil {
		t.Fatalf("ctrl+x %s emitted a command", key)
	}
	return updated.(Model), cmd
}

func TestStatusCommandShowsStableDeterministicPlaceholderSurface(t *testing.T) {
	t.Parallel()

	renderStatus := func(messages ...tea.KeyMsg) string {
		model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
			return TranscriptTurn{UserText: text, AssistantText: "unexpected"}
		}, nil)
		var updated tea.Model = model
		for _, message := range messages {
			var cmd tea.Cmd
			updated, cmd = updated.(Model).Update(message)
			if cmd != nil {
				t.Fatalf("status route emitted unexpected Bubble Tea command")
			}
		}
		return updated.(Model).View()
	}

	slash := renderStatus(
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/status")},
		tea.KeyMsg{Type: tea.KeyEnter},
	)
	shortcut := renderStatus(
		tea.KeyMsg{Type: tea.KeyCtrlX},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")},
	)
	repeated := renderStatus(
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/status")},
		tea.KeyMsg{Type: tea.KeyEnter},
	)

	if slash != repeated {
		t.Fatalf("status render is not stable:\nfirst:\n%s\n\nsecond:\n%s", slash, repeated)
	}
	for _, render := range []string{slash, shortcut} {
		assertOrdered(t, render, "status:", "Deterministic placeholder status.")
		assertOrdered(t, render, "Deterministic placeholder status.", "real status sources: deferred")
		for _, forbidden := range []string{"2026-", "T16:", "timestamp", "time:", "Fake Aila response"} {
			if strings.Contains(render, forbidden) {
				t.Fatalf("status render contains unstable or prompt marker %q:\n%s", forbidden, render)
			}
		}
	}
}

func TestHelpCommandShowsOnlyM5CommandsAndShortcutsInStableOrder(t *testing.T) {
	t.Parallel()

	renderHelp := func() string {
		model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 80, Height: 24}, nil, nil)
		updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/help")})
		if cmd != nil {
			t.Fatal("typing help must not emit a Bubble Tea command")
		}
		updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil {
			t.Fatal("help route emitted unexpected Bubble Tea command")
		}
		return updated.(Model).View()
	}

	first := renderHelp()
	second := renderHelp()
	if first != second {
		t.Fatalf("help render is not stable:\nfirst:\n%s\n\nsecond:\n%s", first, second)
	}
	for _, item := range []string{
		"help:",
		"Deterministic placeholder help.",
		"commands:",
		"/status - Show deterministic placeholder status.",
		"/help - Show this deterministic placeholder help.",
		"/quit - Quit Aila.",
		"shortcuts:",
		"ctrl+x s - Show deterministic placeholder status.",
		"ctrl+x q - Quit Aila.",
	} {
		if !strings.Contains(first, item) {
			t.Fatalf("help render missing %q:\n%s", item, first)
		}
	}
	assertOrdered(t, first, "/status - Show deterministic placeholder status.", "/help - Show this deterministic placeholder help.")
	assertOrdered(t, first, "/help - Show this deterministic placeholder help.", "/quit - Quit Aila.")
	assertOrdered(t, first, "/quit - Quit Aila.", "shortcuts:")
	assertOrdered(t, first, "ctrl+x s - Show deterministic placeholder status.", "ctrl+x q - Quit Aila.")
	for _, forbidden := range []string{
		"/new", "/clear", "/continue", "/review", "/history", "/undo", "/redo", "/diff", "/editor", "/compact", "/model", "/auto", "/exit -", "/q -",
		"ctrl+x n", "ctrl+x c", "ctrl+x i", "ctrl+x h", "ctrl+x u", "ctrl+x r", "ctrl+x d", "ctrl+x e", "ctrl+x k", "ctrl+x m", "ctrl+x a", "ctrl+x ?",
		"2026-", "timestamp", "time:",
	} {
		if strings.Contains(first, forbidden) {
			t.Fatalf("help render contains deferred command, shortcut, or time marker %q:\n%s", forbidden, first)
		}
	}
}

func TestPromptQWhileComposingRemainsInput(t *testing.T) {
	t.Parallel()

	var commands []policy.CommandRecommendation
	model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 80, Height: 24}, nil, func(recommendation policy.CommandRecommendation) {
		commands = append(commands, recommendation)
	})
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hel")})
	if cmd != nil {
		t.Fatal("typing setup must not emit a command")
	}

	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("q while composing must not emit a Bubble Tea command")
	}
	if got.Quitting() {
		t.Fatal("q while composing should not quit")
	}
	if got.PromptInput() != "helq" {
		t.Fatalf("prompt input = %q, want helq", got.PromptInput())
	}
	if len(commands) != 0 {
		t.Fatalf("q while composing routed commands: %+v", commands)
	}
}

func TestPromptEnterKeepsOrdinaryAndUnknownSlashFakeBehavior(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"explain this repo", "/unknown"} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			var routed []string
			var commands []policy.CommandRecommendation
			model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
				routed = append(routed, text)
				return TranscriptTurn{UserText: text, AssistantText: "Fake Aila response: " + text}
			}, func(recommendation policy.CommandRecommendation) {
				commands = append(commands, recommendation)
			})
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(input)})
			if cmd != nil {
				t.Fatal("typing setup must not emit a command")
			}
			updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
			got := updated.(Model)

			if cmd != nil {
				t.Fatal("fake prompt submit must route synchronously without a Bubble Tea command")
			}
			if len(commands) != 0 {
				t.Fatalf("ordinary prompt routed commands: %+v", commands)
			}
			if len(routed) != 1 || routed[0] != input {
				t.Fatalf("routed prompts = %v, want [%s]", routed, input)
			}
			if !containsAll(got.View(), []string{"user: " + input, "assistant: Fake Aila response: " + input}) {
				t.Fatalf("view does not show fake prompt transcript:\n%s", got.View())
			}
		})
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
	assertOrdered(t, first, "user: explain this repo", "assistant: Fake Aila response: explain this repo")
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
			for _, forbidden := range []string{"CommandRouter", "Provider", "Adapter", "exec.Command", "git "} {
				if strings.Contains(string(source), forbidden) {
					t.Fatalf("TUI %s contains future-scope or IO behavior marker %q", file, forbidden)
				}
			}
		})
	}
}

func TestCommandSurfacesStayDeterministicAndIOFree(t *testing.T) {
	t.Parallel()

	for _, file := range []string{"model.go", "render.go"} {
		file := file
		t.Run(file, func(t *testing.T) {
			t.Parallel()

			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read TUI %s: %v", file, err)
			}
			for _, forbidden := range []string{
				"time.Now", "os.Read", "os.Write", "os.Open", "exec.Command", "http.Get", "http.Post", "net.Dial", ".aila", "git status", "git diff",
				"internal/app", "internal/agent", "internal/capability", "internal/permission", "internal/runtime", "internal/state", "internal/tools", "internal/workflow",
			} {
				if strings.Contains(string(source), forbidden) {
					t.Fatalf("TUI %s command surface contains IO or deferred source marker %q", file, forbidden)
				}
			}
		})
	}
}

func TestTUIProductionSourceDoesNotOwnWorkflowPhases(t *testing.T) {
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
			for _, spec := range parsed.Imports {
				if strings.Trim(spec.Path.Value, "\"") == "github.com/jgabor/aila/internal/workflow" {
					t.Fatalf("TUI %s imports workflow vocabulary", file)
				}
			}

			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read TUI %s: %v", file, err)
			}
			for _, forbidden := range []string{
				"PhaseIdle", "PhaseEnvision", "PhaseDeliberate", "PhasePlan", "PhaseBuild", "PhaseAudit",
				"RuntimeStatusWaiting", "RuntimeStatusStuck", "RuntimeStatusFlagged", "ParseRuntimeStatus",
				"waiting", "stuck", "flagged",
				"phaseDisplayLabels", "ParsePhase", "WorkflowTransition", "workflow_transition", "transition table", "Transition(",
			} {
				if strings.Contains(string(source), forbidden) {
					t.Fatalf("TUI %s owns workflow phase or transition marker %q", file, forbidden)
				}
			}
		})
	}
}
