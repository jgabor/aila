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

func TestPromptSubmitAppliesAppOwnedRuntimeStatus(t *testing.T) {
	t.Parallel()

	model := NewModelWithStateSizePromptSubmitAndCommandRoute(IdleEmptyState(), Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
		return TranscriptTurn{
			UserText:      text,
			AssistantText: "Fake Aila response: " + text,
			RuntimeStatus: "idle",
			StatusSource:  "runtime.dispatch",
			StatusDetail:  "fake in-memory runtime loop",
			RuntimeResult: "Fake Aila response: " + text,
		}
	}, nil)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	if cmd != nil {
		t.Fatal("typing prompt emitted a command")
	}
	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("ordinary prompt submit with runtime status must not emit a command")
	}
	if !containsAll(got.View(), []string{"Runtime idle", "status source: runtime.dispatch", "detail: fake in-memory runtime loop", "result: Fake Aila response: hello"}) {
		t.Fatalf("view missing app-owned runtime status:\n%s", got.View())
	}
}

func TestCtrlCEmitsAppInterruptMessageOnly(t *testing.T) {
	t.Parallel()

	var interrupts []string
	var commands []policy.CommandRecommendation
	model := NewModelWithStateSizePromptSubmitCommandRouteAndInterrupt(IdleEmptyState(), Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
		return TranscriptTurn{UserText: text, AssistantText: "unexpected"}
	}, func(recommendation policy.CommandRecommendation, state ViewState) ViewState {
		commands = append(commands, recommendation)
		return state
	}, func(reason string) TranscriptTurn {
		interrupts = append(interrupts, reason)
		return TranscriptTurn{
			RuntimeStatus: "canceling",
			StatusSource:  "runtime.dispatch",
			StatusDetail:  "fake in-memory runtime loop",
			RuntimeActive: true,
		}
	})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	got := updated.(Model)

	if cmd != nil {
		t.Fatal("ctrl-c interrupt must not emit a Bubble Tea command")
	}
	if got.Quitting() {
		t.Fatal("ctrl-c interrupt must not quit or cancel from the TUI")
	}
	if len(interrupts) != 1 || interrupts[0] != "ctrl-c" {
		t.Fatalf("interrupt requests = %#v, want ctrl-c", interrupts)
	}
	if len(commands) != 0 {
		t.Fatalf("ctrl-c routed command recommendations: %+v", commands)
	}
	if got.state.RuntimeStatus != "canceling" || !got.state.RuntimeActive {
		t.Fatalf("runtime display state = %+v, want injected canceling active state", got.state)
	}
}

func TestCtrlXCEmitsContinueCommandWithoutInterrupt(t *testing.T) {
	t.Parallel()

	var interrupts []string
	var prompts []string
	var commands []policy.CommandRecommendation
	model := NewModelWithStateSizePromptSubmitCommandRouteAndInterrupt(IdleEmptyState(), Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
		prompts = append(prompts, text)
		return TranscriptTurn{UserText: text, AssistantText: "unexpected"}
	}, func(recommendation policy.CommandRecommendation, state ViewState) ViewState {
		commands = append(commands, recommendation)
		return state
	}, func(reason string) TranscriptTurn {
		interrupts = append(interrupts, reason)
		return TranscriptTurn{RuntimeStatus: "canceling", RuntimeActive: true}
	})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if cmd != nil {
		t.Fatal("ctrl+x prefix must not emit a Bubble Tea command")
	}
	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	got := updated.(Model)

	if cmd != nil {
		t.Fatal("ctrl+x c continue command must not emit a Bubble Tea command")
	}
	if got.Quitting() {
		t.Fatal("ctrl+x c continue must not quit")
	}
	if len(interrupts) != 0 {
		t.Fatalf("ctrl+x c routed interrupts: %#v", interrupts)
	}
	if len(prompts) != 0 {
		t.Fatalf("ctrl+x c routed prompts: %#v", prompts)
	}
	if len(commands) != 1 || commands[0].Route != policy.CommandRouteContinue || commands[0].Kind != policy.CommandInputShortcut {
		t.Fatalf("commands = %+v, want shortcut continue", commands)
	}
	if got.state.Session == nil || got.state.Session.Action != "continue" {
		t.Fatalf("ctrl+x c state session = %+v, want continue surface", got.state.Session)
	}
}

func TestPromptEnterRoutesRecognizedCommandThroughPolicyBoundary(t *testing.T) {
	t.Parallel()

	var routed []string
	var commands []policy.CommandRecommendation
	model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 80, Height: 24}, func(text string) TranscriptTurn {
		routed = append(routed, text)
		return TranscriptTurn{UserText: text, AssistantText: "Fake Aila response: " + text}
	}, func(recommendation policy.CommandRecommendation, state ViewState) ViewState {
		commands = append(commands, recommendation)
		return state
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
	}, func(recommendation policy.CommandRecommendation, state ViewState) ViewState {
		commands = append(commands, recommendation)
		return state
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

	newSlash, ok := policy.RecommendSlashCommand("/new")
	if !ok {
		t.Fatal("/new did not match")
	}
	newShortcut, ok := policy.RecommendShortcut("ctrl+x", "n")
	if !ok {
		t.Fatal("ctrl+x n did not match")
	}
	if newSlash.Route != newShortcut.Route || newSlash.Route != policy.CommandRouteNew {
		t.Fatalf("new policy route mismatch: slash=%+v shortcut=%+v", newSlash, newShortcut)
	}
	clearSlash, ok := policy.RecommendSlashCommand("/clear")
	if !ok || clearSlash.Route != policy.CommandRouteClear {
		t.Fatalf("/clear route = %+v matched=%v, want slash clear", clearSlash, ok)
	}
	continueSlash, ok := policy.RecommendSlashCommand("/continue")
	if !ok {
		t.Fatal("/continue did not match")
	}
	continueShortcut, ok := policy.RecommendShortcut("ctrl+x", "c")
	if !ok {
		t.Fatal("ctrl+x c did not match")
	}
	if continueSlash.Route != continueShortcut.Route || continueSlash.Route != policy.CommandRouteContinue {
		t.Fatalf("continue policy route mismatch: slash=%+v shortcut=%+v", continueSlash, continueShortcut)
	}

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

	undoSlash, ok := policy.RecommendSlashCommand("/undo")
	if !ok {
		t.Fatal("/undo did not match")
	}
	undoShortcut, ok := policy.RecommendShortcut("ctrl+x", "u")
	if !ok {
		t.Fatal("ctrl+x u did not match")
	}
	if undoSlash.Route != undoShortcut.Route || undoSlash.Route != policy.CommandRouteUndo {
		t.Fatalf("undo policy route mismatch: slash=%+v shortcut=%+v", undoSlash, undoShortcut)
	}

	redoSlash, ok := policy.RecommendSlashCommand("/redo")
	if !ok {
		t.Fatal("/redo did not match")
	}
	redoShortcut, ok := policy.RecommendShortcut("ctrl+x", "r")
	if !ok {
		t.Fatal("ctrl+x r did not match")
	}
	if redoSlash.Route != redoShortcut.Route || redoSlash.Route != policy.CommandRouteRedo {
		t.Fatalf("redo policy route mismatch: slash=%+v shortcut=%+v", redoSlash, redoShortcut)
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

	newSlashModel, newSlashCmd := routeSlashCommandForParity(t, "/new")
	newShortcutModel, newShortcutCmd := routeShortcutForParity(t, "n")
	if newSlashCmd != nil || newShortcutCmd != nil {
		t.Fatal("new parity routes should not emit Bubble Tea commands")
	}
	if newSlashModel.state.CommandRoute != newShortcutModel.state.CommandRoute || newSlashModel.state.RouteSource != newShortcutModel.state.RouteSource {
		t.Fatalf("new TUI route mismatch: slash=%+v shortcut=%+v", newSlashModel.state, newShortcutModel.state)
	}
	if newSlashModel.state.Session == nil || newShortcutModel.state.Session == nil || newSlashModel.state.Session.Action != "new" || newShortcutModel.state.Session.Action != "new" {
		t.Fatalf("new session surfaces missing: slash=%+v shortcut=%+v", newSlashModel.state.Session, newShortcutModel.state.Session)
	}
	continueSlashModel, continueSlashCmd := routeSlashCommandForParity(t, "/continue")
	continueShortcutModel, continueShortcutCmd := routeShortcutForParity(t, "c")
	if continueSlashCmd != nil || continueShortcutCmd != nil {
		t.Fatal("continue parity routes should not emit Bubble Tea commands")
	}
	if continueSlashModel.state.CommandRoute != continueShortcutModel.state.CommandRoute || continueSlashModel.state.RouteSource != continueShortcutModel.state.RouteSource {
		t.Fatalf("continue TUI route mismatch: slash=%+v shortcut=%+v", continueSlashModel.state, continueShortcutModel.state)
	}
	if continueSlashModel.state.Session == nil || continueShortcutModel.state.Session == nil || continueSlashModel.state.Session.Action != "continue" || continueShortcutModel.state.Session.Action != "continue" {
		t.Fatalf("continue session surfaces missing: slash=%+v shortcut=%+v", continueSlashModel.state.Session, continueShortcutModel.state.Session)
	}
	clearSlashModel, clearSlashCmd := routeSlashCommandForParity(t, "/clear")
	if clearSlashCmd != nil {
		t.Fatal("clear slash route should not emit Bubble Tea commands")
	}
	if clearSlashModel.state.Session == nil || clearSlashModel.state.Session.Action != "clear" {
		t.Fatalf("clear session surface missing: %+v", clearSlashModel.state.Session)
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
	reviewSlash, ok := policy.RecommendSlashCommand("/review")
	if !ok {
		t.Fatal("/review did not match")
	}
	reviewShortcut, ok := policy.RecommendShortcut("ctrl+x", "i")
	if !ok {
		t.Fatal("ctrl+x i did not match")
	}
	if reviewSlash.Route != reviewShortcut.Route || reviewSlash.Route != policy.CommandRouteReview {
		t.Fatalf("review policy route mismatch: slash=%+v shortcut=%+v", reviewSlash, reviewShortcut)
	}
	reviewSlashModel, reviewSlashCmd := routeSlashCommandForParity(t, "/review")
	reviewShortcutModel, reviewShortcutCmd := routeShortcutForParity(t, "i")
	if reviewSlashCmd != nil || reviewShortcutCmd != nil {
		t.Fatal("review parity routes should not emit Bubble Tea commands")
	}
	if reviewSlashModel.state.CommandRoute != reviewShortcutModel.state.CommandRoute || reviewSlashModel.state.RouteSource != reviewShortcutModel.state.RouteSource {
		t.Fatalf("review TUI route mismatch: slash=%+v shortcut=%+v", reviewSlashModel.state, reviewShortcutModel.state)
	}
	if reviewSlashModel.state.SurfaceTitle != "review" || reviewShortcutModel.state.SurfaceTitle != "review" {
		t.Fatalf("review surface mismatch: slash=%q shortcut=%q", reviewSlashModel.state.SurfaceTitle, reviewShortcutModel.state.SurfaceTitle)
	}
	historySlash, ok := policy.RecommendSlashCommand("/history")
	if !ok {
		t.Fatal("/history did not match")
	}
	historyShortcut, ok := policy.RecommendShortcut("ctrl+x", "h")
	if !ok {
		t.Fatal("ctrl+x h did not match")
	}
	if historySlash.Route != historyShortcut.Route || historySlash.Route != policy.CommandRouteHistory {
		t.Fatalf("history policy route mismatch: slash=%+v shortcut=%+v", historySlash, historyShortcut)
	}
	historySlashModel, historySlashCmd := routeSlashCommandForParity(t, "/history")
	historyShortcutModel, historyShortcutCmd := routeShortcutForParity(t, "h")
	if historySlashCmd != nil || historyShortcutCmd != nil {
		t.Fatal("history parity routes should not emit Bubble Tea commands")
	}
	if historySlashModel.state.CommandRoute != historyShortcutModel.state.CommandRoute || historySlashModel.state.RouteSource != historyShortcutModel.state.RouteSource {
		t.Fatalf("history TUI route mismatch: slash=%+v shortcut=%+v", historySlashModel.state, historyShortcutModel.state)
	}
	if historySlashModel.state.SurfaceTitle != "history" || historyShortcutModel.state.SurfaceTitle != "history" {
		t.Fatalf("history surface mismatch: slash=%q shortcut=%q", historySlashModel.state.SurfaceTitle, historyShortcutModel.state.SurfaceTitle)
	}

	diffSlash, ok := policy.RecommendSlashCommand("/diff")
	if !ok {
		t.Fatal("/diff did not match")
	}
	diffShortcut, ok := policy.RecommendShortcut("ctrl+x", "d")
	if !ok {
		t.Fatal("ctrl+x d did not match")
	}
	if diffSlash.Route != diffShortcut.Route || diffSlash.Route != policy.CommandRouteDiff {
		t.Fatalf("diff policy route mismatch: slash=%+v shortcut=%+v", diffSlash, diffShortcut)
	}
	diffSlashModel, diffSlashCmd := routeSlashCommandForParity(t, "/diff")
	diffShortcutModel, diffShortcutCmd := routeShortcutForParity(t, "d")
	if diffSlashCmd != nil || diffShortcutCmd != nil {
		t.Fatal("diff parity routes should not emit Bubble Tea commands")
	}
	if diffSlashModel.state.CommandRoute != diffShortcutModel.state.CommandRoute || diffSlashModel.state.RouteSource != diffShortcutModel.state.RouteSource {
		t.Fatalf("diff TUI route mismatch: slash=%+v shortcut=%+v", diffSlashModel.state, diffShortcutModel.state)
	}
	if diffSlashModel.state.SurfaceTitle != "diff" || diffShortcutModel.state.SurfaceTitle != "diff" || !diffSlashModel.state.DiffFocus || !diffShortcutModel.state.DiffFocus {
		t.Fatalf("diff surface mismatch: slash=%+v shortcut=%+v", diffSlashModel.state, diffShortcutModel.state)
	}

	undoSlashModel, undoSlashCmd := routeSlashCommandForParity(t, "/undo")
	undoShortcutModel, undoShortcutCmd := routeShortcutForParity(t, "u")
	if undoSlashCmd != nil || undoShortcutCmd != nil {
		t.Fatal("undo parity routes should not emit Bubble Tea commands")
	}
	if undoSlashModel.state.CommandRoute != undoShortcutModel.state.CommandRoute || undoSlashModel.state.RouteSource != undoShortcutModel.state.RouteSource {
		t.Fatalf("undo TUI route mismatch: slash=%+v shortcut=%+v", undoSlashModel.state, undoShortcutModel.state)
	}

	redoSlashModel, redoSlashCmd := routeSlashCommandForParity(t, "/redo")
	redoShortcutModel, redoShortcutCmd := routeShortcutForParity(t, "r")
	if redoSlashCmd != nil || redoShortcutCmd != nil {
		t.Fatal("redo parity routes should not emit Bubble Tea commands")
	}
	if redoSlashModel.state.CommandRoute != redoShortcutModel.state.CommandRoute || redoSlashModel.state.RouteSource != redoShortcutModel.state.RouteSource {
		t.Fatalf("redo TUI route mismatch: slash=%+v shortcut=%+v", redoSlashModel.state, redoShortcutModel.state)
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

func TestStatusCommandFallbackShowsPresentationOnlyState(t *testing.T) {
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
		assertOrdered(t, render, "status:", "app-owned status inspection unavailable in presentation-only fallback")
		assertOrdered(t, render, "app-owned status inspection unavailable in presentation-only fallback", "stage:")
		for _, forbidden := range []string{"2026-", "T16:", "timestamp", "time:", "Fake Aila response"} {
			if strings.Contains(render, forbidden) {
				t.Fatalf("status render contains unstable or prompt marker %q:\n%s", forbidden, render)
			}
		}
	}
}

func TestHelpCommandShowsUndoRedoCommandsAndShortcutsInStableOrder(t *testing.T) {
	t.Parallel()

	renderHelp := func() string {
		model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 160, Height: 45}, nil, nil)
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
		"/new - Start a fresh session and preserve project memory.",
		"/clear - Clear visible session state and current memory.",
		"/continue - Restore the current saved session.",
		"/status - Inspect current runtime and state.",
		"/review - Inspect current changes, risks, and sources.",
		"/history - Browse runs, edits, checks, and undo data.",
		"/help - Show this deterministic placeholder help.",
		"/diff - Review current changes.",
		"/undo - Undo the latest supported mutation.",
		"/redo - Redo the latest supported recovery.",
		"/quit - Quit Aila.",
		"shortcuts:",
		"ctrl+x n - Start a fresh session and preserve project memory.",
		"ctrl+x c - Restore the current saved session.",
		"ctrl+x s - Inspect current runtime and state.",
		"ctrl+x i - Inspect current changes, risks, and sources.",
		"ctrl+x h - Browse runs, edits, checks, and undo data.",
		"ctrl+x d - Review current changes.",
		"ctrl+x u - Undo the latest supported mutation.",
		"ctrl+x r - Redo the latest supported recovery.",
		"ctrl+x q - Quit Aila.",
	} {
		if !strings.Contains(first, item) {
			t.Fatalf("help render missing %q:\n%s", item, first)
		}
	}
	assertOrdered(t, first, "/new - Start a fresh session and preserve project memory.", "/clear - Clear visible session state and current memory.")
	assertOrdered(t, first, "/clear - Clear visible session state and current memory.", "/continue - Restore the current saved session.")
	assertOrdered(t, first, "/continue - Restore the current saved session.", "/status - Inspect current runtime and state.")
	assertOrdered(t, first, "/status - Inspect current runtime and state.", "/review - Inspect current changes, risks, and sources.")
	assertOrdered(t, first, "/review - Inspect current changes, risks, and sources.", "/history - Browse runs, edits, checks, and undo data.")
	assertOrdered(t, first, "/history - Browse runs, edits, checks, and undo data.", "/help - Show this deterministic placeholder help.")
	assertOrdered(t, first, "/help - Show this deterministic placeholder help.", "/diff - Review current changes.")
	assertOrdered(t, first, "/diff - Review current changes.", "/undo - Undo the latest supported mutation.")
	assertOrdered(t, first, "/undo - Undo the latest supported mutation.", "/redo - Redo the latest supported recovery.")
	assertOrdered(t, first, "/redo - Redo the latest supported recovery.", "/quit - Quit Aila.")
	assertOrdered(t, first, "/quit - Quit Aila.", "shortcuts:")
	assertOrdered(t, first, "ctrl+x n - Start a fresh session and preserve project memory.", "ctrl+x c - Restore the current saved session.")
	assertOrdered(t, first, "ctrl+x c - Restore the current saved session.", "ctrl+x s - Inspect current runtime and state.")
	assertOrdered(t, first, "ctrl+x s - Inspect current runtime and state.", "ctrl+x i - Inspect current changes, risks, and sources.")
	assertOrdered(t, first, "ctrl+x i - Inspect current changes, risks, and sources.", "ctrl+x h - Browse runs, edits, checks, and undo data.")
	assertOrdered(t, first, "ctrl+x h - Browse runs, edits, checks, and undo data.", "ctrl+x d - Review current changes.")
	assertOrdered(t, first, "ctrl+x d - Review current changes.", "ctrl+x u - Undo the latest supported mutation.")
	assertOrdered(t, first, "ctrl+x u - Undo the latest supported mutation.", "ctrl+x r - Redo the latest supported recovery.")
	assertOrdered(t, first, "ctrl+x r - Redo the latest supported recovery.", "ctrl+x q - Quit Aila.")
	for _, forbidden := range []string{
		"/editor", "/compact", "/model", "/auto", "/exit -", "/q -",
		"ctrl+x e", "ctrl+x k", "ctrl+x m", "ctrl+x a", "ctrl+x ?",
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
	model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 80, Height: 24}, nil, func(recommendation policy.CommandRecommendation, state ViewState) ViewState {
		commands = append(commands, recommendation)
		return state
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
			}, func(recommendation policy.CommandRecommendation, state ViewState) ViewState {
				commands = append(commands, recommendation)
				return state
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

func TestSessionSurfaceSelectionFocusIsPresentationOnly(t *testing.T) {
	t.Parallel()

	state := ApplySessionView(IdleEmptyState(), &SessionView{
		Action:       "continue",
		Source:       "app.session",
		Status:       "loaded",
		SessionID:    "current",
		MemoryStatus: "visible",
		Detail:       "restored current session snapshot",
		Focus:        true,
		Items: []SessionItemView{
			{ID: "current", Status: "loaded", MemoryStatus: "visible", Detail: "current session"},
			{ID: "previous", Status: "available", MemoryStatus: "visible", Detail: "injected row"},
		},
	})
	model := NewModelWithStateSizePromptSubmitAndCommandRoute(state, Size{Width: 80, Height: 24}, nil, func(recommendation policy.CommandRecommendation, state ViewState) ViewState {
		t.Fatalf("session focus should not route app commands: %+v", recommendation)
		return state
	})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("session down navigation emitted a command")
	}
	if got.state.Session == nil || got.state.Session.Selected != 1 || !got.state.Session.Focus {
		t.Fatalf("session selection after down = %+v, want focused second row", got.state.Session)
	}
	semantic := Semantic(got.state, Size{Width: 80, Height: 24})
	if semantic.Screen.Focus != "session" || semantic.SessionView == nil || semantic.SessionView.Selected != 1 || !semantic.SessionView.Focus {
		t.Fatalf("session semantic after down = focus %q view %+v", semantic.Screen.Focus, semantic.SessionView)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("session enter emitted a command")
	}
	if got.state.Session == nil || got.state.Session.Focus {
		t.Fatalf("session enter should release focus: %+v", got.state.Session)
	}
}

func TestHistoryViewNavigationSelectionFocusEmptyAndBoundsAreDeterministic(t *testing.T) {
	t.Parallel()

	items := make([]HistoryItem, 0, 20)
	for i := 0; i < 20; i++ {
		items = append(items, HistoryItem{
			EventID:     "event-" + string(rune('a'+i)),
			RunID:       "run-1",
			SessionID:   "session-1",
			Kind:        "prompt",
			Source:      "user",
			Provenance:  "prompt.submit",
			DisplayText: "entry " + string(rune('a'+i)),
		})
	}
	state := ApplyHistoryView(IdleEmptyState(), items, 0, true)
	model := NewModelWithStateSizePromptSubmitAndCommandRoute(state, Size{Width: 80, Height: 24}, nil, nil)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("history up navigation emitted a command")
	}
	if got.state.HistorySelected != 0 {
		t.Fatalf("history selection after up at start = %d, want 0", got.state.HistorySelected)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("history page down emitted a command")
	}
	if got.state.HistorySelected != 12 {
		t.Fatalf("history selection after page down = %d, want 12", got.state.HistorySelected)
	}
	if !strings.Contains(strings.Join(got.state.SurfaceLines, "\n"), "event-m") || strings.Contains(strings.Join(got.state.SurfaceLines, "\n"), "event-a") {
		t.Fatalf("bounded history window did not follow selected row: %#v", got.state.SurfaceLines)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEnd})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("history end emitted a command")
	}
	if got.state.HistorySelected != 19 {
		t.Fatalf("history selection after end = %d, want 19", got.state.HistorySelected)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("history down at end emitted a command")
	}
	if got.state.HistorySelected != 19 {
		t.Fatalf("history selection after down at end = %d, want 19", got.state.HistorySelected)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("history escape emitted a command")
	}
	if got.state.HistoryFocus {
		t.Fatal("history escape should release focus")
	}
}

func TestHistoryViewEmptyStateIsDeterministic(t *testing.T) {
	t.Parallel()

	state := ApplyHistoryView(IdleEmptyState(), nil, 4, true)
	model := NewModelWithStateSizePromptSubmitAndCommandRoute(state, Size{Width: 80, Height: 24}, nil, nil)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)

	if cmd != nil {
		t.Fatal("empty history navigation emitted a command")
	}
	if got.state.HistorySelected != 0 || !got.state.HistoryEmpty || !got.state.HistoryFocus {
		t.Fatalf("empty history state = %+v, want clamped focused empty history", got.state)
	}
	if !containsAll(got.View(), []string{"history:", "read-only: true", "empty history", "no fake history events recorded yet"}) {
		t.Fatalf("empty history render missing deterministic lines:\n%s", got.View())
	}
	semantic := Semantic(got.state, Size{Width: 80, Height: 24})
	if semantic.History == nil || !semantic.History.ReadOnly || !semantic.History.Empty || semantic.History.Count != 0 {
		t.Fatalf("empty history semantic = %+v", semantic.History)
	}
}

func TestDiffViewNavigationSelectionFocusExitAreDeterministic(t *testing.T) {
	t.Parallel()

	state := ApplyDiffView(IdleEmptyState(), diffNavigationView(), 0, true)
	model := NewModelWithStateSizePromptSubmitAndCommandRoute(state, Size{Width: 80, Height: 24}, nil, nil)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("diff up navigation emitted a command")
	}
	if got.state.DiffSelected != 0 {
		t.Fatalf("diff selection after up at start = %d, want 0", got.state.DiffSelected)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("diff page down emitted a command")
	}
	if got.state.DiffSelected != 12 {
		t.Fatalf("diff selection after page down = %d, want 12", got.state.DiffSelected)
	}
	if !strings.Contains(strings.Join(got.state.SurfaceLines, "\n"), "selected: 13") {
		t.Fatalf("bounded diff window did not record selected row: %#v", got.state.SurfaceLines)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEnd})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("diff end emitted a command")
	}
	if got.state.DiffSelected != 19 {
		t.Fatalf("diff selection after end = %d, want 19", got.state.DiffSelected)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("diff down at end emitted a command")
	}
	if got.state.DiffSelected != 19 {
		t.Fatalf("diff selection after down at end = %d, want 19", got.state.DiffSelected)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyHome})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("diff home emitted a command")
	}
	if got.state.DiffSelected != 0 {
		t.Fatalf("diff selection after home = %d, want 0", got.state.DiffSelected)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("diff escape emitted a command")
	}
	if got.state.Diff != nil || got.state.DiffFocus || got.state.SurfaceTitle == "diff" || got.state.CommandRoute == "diff" {
		t.Fatalf("diff escape should clear diff surface: %+v", got.state)
	}
}

func TestDiffViewEmptyStateIsDeterministic(t *testing.T) {
	t.Parallel()

	state := ApplyDiffView(IdleEmptyState(), &DiffView{Source: "app.diff", Status: "empty", Empty: true}, 4, true)
	model := NewModelWithStateSizePromptSubmitAndCommandRoute(state, Size{Width: 80, Height: 24}, nil, nil)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)

	if cmd != nil {
		t.Fatal("empty diff navigation emitted a command")
	}
	if got.state.DiffSelected != 0 || got.state.Diff == nil || !got.state.Diff.Empty || !got.state.DiffFocus {
		t.Fatalf("empty diff state = %+v, want clamped focused empty diff", got.state)
	}
	if !containsAll(got.View(), []string{"diff:", "read-only: true", "source: app.diff", "status: empty", "no changes"}) {
		t.Fatalf("empty diff render missing deterministic lines:\n%s", got.View())
	}
	semantic := Semantic(got.state, Size{Width: 80, Height: 24})
	if semantic.Diff == nil || !semantic.Diff.ReadOnly || !semantic.Diff.Empty || semantic.Diff.FileCount != 0 {
		t.Fatalf("empty diff semantic = %+v", semantic.Diff)
	}
}

func diffNavigationView() *DiffView {
	lines := make([]DiffLineView, 0, 18)
	for i := 0; i < 18; i++ {
		if i%2 == 0 {
			lines = append(lines, DiffLineView{Kind: "removal", Text: "old line " + string(rune('a'+i)), OldLine: i + 1})
			continue
		}
		lines = append(lines, DiffLineView{Kind: "addition", Text: "new line " + string(rune('a'+i)), NewLine: i + 1})
	}
	return &DiffView{
		Source: "app.diff",
		Status: "ready",
		Files: []DiffFileView{{
			Path:   "internal/demo.txt",
			Status: "modified",
			Hunks: []DiffHunkView{{
				Header:   "@@ -1,18 +1,18 @@",
				OldStart: 1,
				OldLines: 18,
				NewStart: 1,
				NewLines: 18,
				Lines:    lines,
			}},
		}},
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

func TestTUIProductionSourceRendersInjectedRuntimeStatusOnly(t *testing.T) {
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
				if strings.Trim(spec.Path.Value, "\"") == "github.com/jgabor/aila/internal/runtime" {
					t.Fatalf("TUI %s imports runtime instead of rendering injected status fields", file)
				}
			}

			source, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read TUI %s: %v", file, err)
			}
			for _, forbidden := range []string{
				"runtime.Update", "runtime.Dispatch", "PromptSubmitted", "CommandSelected", "FakeEffectCompleted", "FakeEffectFailed",
				"FakePromptEffect", "FakeCommandEffect", "OperationMetadata", "[]runtime.Effect", "[]runtime.Message",
			} {
				if strings.Contains(string(source), forbidden) {
					t.Fatalf("TUI %s owns runtime update/effect decision marker %q", file, forbidden)
				}
			}
		})
	}
}
