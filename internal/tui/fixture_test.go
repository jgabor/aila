package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jgabor/aila/internal/policy"
)

func TestRequiredM3FixtureSet(t *testing.T) {
	t.Parallel()

	required := map[string][]fixtureSize{
		"idle-empty":   m6FixtureSizes(),
		"narrow-80":    m6FixtureSizes(),
		"desktop-wide": m6FixtureSizes(),
	}

	for name, sizes := range required {
		name := name
		sizes := sizes
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadStaticShellFixture(t, name)
			if fixture.Name != name {
				t.Fatalf("fixture name = %q, want %s", fixture.Name, name)
			}
			if fixture.Kind != "static_shell" {
				t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
			}
			if fixture.TerminalBehavior != "bubbletea_static" {
				t.Fatalf("terminal behavior = %q, want bubbletea_static", fixture.TerminalBehavior)
			}
			if fixture.QuitInput != "q" {
				t.Fatalf("quit input = %q, want q", fixture.QuitInput)
			}
			if len(fixture.States) != 1 || fixture.States[0] != "idle" {
				t.Fatalf("fixture states = %v, want [idle]", fixture.States)
			}
			assertFixtureSizes(t, fixture, sizes)
			for _, size := range fixture.Sizes {
				for _, kind := range []string{"plain", "ansi", "semantic"} {
					key := fixtureOutputKey(kind, size.Name)
					if fixture.Outputs[key] == "" {
						t.Fatalf("render output %q missing from fixture metadata", key)
					}
					fixture.ReadFile(t, fixture.Outputs[key])
				}
			}
		})
	}
}

func assertFixtureSizes(t *testing.T, fixture renderFixture, want []fixtureSize) {
	t.Helper()

	if len(fixture.Sizes) != len(want) {
		t.Fatalf("fixture sizes = %+v, want %+v", fixture.Sizes, want)
	}
	for i, size := range fixture.Sizes {
		if size != want[i] {
			t.Fatalf("fixture size %d = %+v, want %+v", i, size, want[i])
		}
	}
}

func loadStaticShellFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Scenario = name
	return loadRenderFixture(t, name, state)
}

func loadSubmittedPromptFixture(t *testing.T) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Scenario = "submitted-prompt"
	state.Transcript = []TranscriptTurn{{
		UserText:      "explain this repo",
		AssistantText: "Fake Aila response: explain this repo",
	}}
	return loadRenderFixture(t, state.Scenario, state)
}

func loadCommandFixture(t *testing.T, name string, input string) renderFixture {
	t.Helper()

	model := NewModelWithSizePromptSubmitAndCommandRoute(Size{Width: 80, Height: 24}, nil, nil)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(input)})
	if cmd != nil {
		t.Fatalf("typing %s emitted a command", input)
	}
	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("routing %s emitted a command", input)
	}
	state := updated.(Model).state
	state.Scenario = name
	return loadRenderFixture(t, name, state)
}

func TestM5CommandFixtureSet(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input string
		route policy.CommandRoute
	}{
		{name: "status-command", input: "/status", route: policy.CommandRouteStatus},
		{name: "help-command", input: "/help", route: policy.CommandRouteHelp},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := loadCommandFixture(t, tc.name, tc.input)
			if fixture.Kind != "static_shell" {
				t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
			}
			if fixture.TerminalBehavior != "bubbletea_static" {
				t.Fatalf("terminal behavior = %q, want bubbletea_static", fixture.TerminalBehavior)
			}
			assertFixtureSizes(t, fixture, m6FixtureSizes())

			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := renderCase.render(fixture.State, renderCase.size)
					assertTextSnapshot(t, fixture, renderCase.file, got)
					assertOrdered(t, got, string(tc.route)+":", "command route: "+string(tc.route))
					assertOrdered(t, got, "command route: "+string(tc.route), "route source: policy.command")
					assertOrdered(t, got, "route source: policy.command", "Deterministic placeholder")
				})
			}

			for _, semanticCase := range fixture.SemanticCases() {
				semanticCase := semanticCase
				t.Run(semanticCase.name, func(t *testing.T) {
					t.Parallel()

					got := RenderSemanticJSON(fixture.State, semanticCase.size)
					assertSemanticSnapshot(t, fixture, semanticCase.file, got)

					var snapshot SemanticSnapshot
					if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
						t.Fatalf("unmarshal semantic snapshot: %v", err)
					}
					assertCommandSemanticContract(t, tc.name, semanticCase.size, string(tc.route), snapshot)
				})
			}
		})
	}
}

func assertCommandSemanticContract(t *testing.T, scenario string, size Size, route string, snapshot SemanticSnapshot) {
	t.Helper()

	assertSemanticContract(t, scenario, size, snapshot)
	if snapshot.Command == nil {
		t.Fatal("command semantic metadata is missing")
	}
	if snapshot.Command.Route != route || snapshot.Command.RouteSource != "policy.command" || snapshot.Command.Surface != route || !snapshot.Command.Visible {
		t.Fatalf("command metadata = %+v, want visible %s from policy.command", *snapshot.Command, route)
	}
	if snapshot.Command.Executed || snapshot.Command.WorkflowTransition {
		t.Fatalf("command metadata implies execution or workflow transition: %+v", *snapshot.Command)
	}
	if snapshot.Screen.Focus != "prompt" {
		t.Fatalf("focus = %q, want prompt", snapshot.Screen.Focus)
	}
	regions := semanticRegionsByName(t, snapshot)
	command, ok := regions["command"]
	if !ok {
		t.Fatal("command region missing")
	}
	if !containsAll(strings.Join(command.Items, "\n"), []string{route, "command route: " + route, "route source: policy.command", "Deterministic placeholder"}) {
		t.Fatalf("command region items = %v, want route, source, and placeholder content", command.Items)
	}
}

func TestM4SubmittedPromptRenderSnapshots(t *testing.T) {
	t.Parallel()

	fixture := loadSubmittedPromptFixture(t)
	if fixture.Kind != "static_shell" {
		t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
	}
	if fixture.TerminalBehavior != "bubbletea_static" {
		t.Fatalf("terminal behavior = %q, want bubbletea_static", fixture.TerminalBehavior)
	}
	assertFixtureSizes(t, fixture, m6FixtureSizes())

	for _, tc := range fixture.TextCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.render(fixture.State, tc.size)
			assertTextSnapshot(t, fixture, tc.file, got)
			assertOrdered(t, got, "user: explain this repo", "assistant: Fake Aila response: explain this repo")
			if !containsAll(got, []string{"Prompt", ">"}) {
				t.Fatalf("submitted prompt fixture should show cleared prompt state:\n%s", got)
			}
		})
	}
}

func TestM6FixtureSnapshotMatrix(t *testing.T) {
	t.Parallel()

	for _, fixture := range currentRenderFixtures(t) {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			assertFixtureSizes(t, fixture, m6FixtureSizes())
			for _, size := range fixture.Sizes {
				for _, kind := range []string{"plain", "ansi", "semantic"} {
					key := fixtureOutputKey(kind, size.Name)
					file := fixture.Outputs[key]
					if file == "" {
						t.Fatalf("render output %q missing from fixture metadata", key)
					}
					fixture.ReadFile(t, file)
				}
			}
		})
	}
}

func m6FixtureSizes() []fixtureSize {
	return []fixtureSize{
		{Name: "80x24", Width: 80, Height: 24},
		{Name: "100x30", Width: 100, Height: 30},
		{Name: "120x32", Width: 120, Height: 32},
		{Name: "160x45", Width: 160, Height: 45},
	}
}

func TestM4SubmittedPromptSemanticSnapshot(t *testing.T) {
	t.Parallel()

	fixture := loadSubmittedPromptFixture(t)
	for _, tc := range fixture.SemanticCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := RenderSemanticJSON(fixture.State, tc.size)
			assertSemanticSnapshot(t, fixture, tc.file, got)

			var snapshot SemanticSnapshot
			if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
				t.Fatalf("unmarshal semantic snapshot: %v", err)
			}
			assertSemanticContract(t, "submitted-prompt", tc.size, snapshot)

			regions := semanticRegionsByName(t, snapshot)
			assertOrdered(t, strings.Join(regions["chat"].Items, "\n"), "user: explain this repo", "assistant: Fake Aila response: explain this repo")
			if got := regions["prompt"].Items; len(got) != 1 || got[0] != ">" {
				t.Fatalf("prompt region items = %v, want cleared prompt marker", got)
			}
			if len(snapshot.Actions) != 1 || snapshot.Actions[0].Name != "quit" || snapshot.Actions[0].Input != "q" {
				t.Fatalf("actions = %+v, want q quit only with no M5 routing semantics", snapshot.Actions)
			}
		})
	}
}

func semanticRegionsByName(t *testing.T, snapshot SemanticSnapshot) map[string]SemanticRegion {
	t.Helper()

	regions := map[string]SemanticRegion{}
	for _, region := range snapshot.Regions {
		regions[region.Name] = region
	}
	return regions
}

func TestRequiredM3FixtureScopes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		size      Size
		wantRail  bool
		forbidden []string
	}{
		{name: "narrow-80", size: Size{Width: 80, Height: 24}, forbidden: []string{"submit", "slash", "command", "right rail", "resize"}},
		{name: "desktop-wide", size: Size{Width: 160, Height: 45}, wantRail: true, forbidden: []string{"submit", "slash", "command", "resize"}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := loadStaticShellFixture(t, tc.name)
			plain := RenderPlain(fixture.State, tc.size)
			if !containsAll(plain, []string{"Conversation", "No messages yet.", "Prompt", ">", "q quit"}) {
				t.Fatalf("%s plain render does not represent the current static shell:\n%s", tc.name, plain)
			}
			if containsAny(plain, tc.forbidden) {
				t.Fatalf("%s plain render includes deferred behavior:\n%s", tc.name, plain)
			}
			if tc.wantRail && !containsAll(plain, []string{"Session", "phase source: not_started", "primary model: placeholder"}) {
				t.Fatalf("%s plain render missing M6 placeholder rail:\n%s", tc.name, plain)
			}

			semantic := Semantic(fixture.State, tc.size)
			if semantic.Screen.Width != tc.size.Width || semantic.Screen.Height != tc.size.Height {
				t.Fatalf("%s screen = %dx%d, want %dx%d", tc.name, semantic.Screen.Width, semantic.Screen.Height, tc.size.Width, tc.size.Height)
			}
			if len(semantic.Actions) != 1 || semantic.Actions[0].Input != "q" {
				t.Fatalf("%s actions = %+v, want single q quit action", tc.name, semantic.Actions)
			}
			for _, region := range semantic.Regions {
				if !tc.wantRail && (region.Name == "right_rail" || region.Name == "right-rail") {
					t.Fatalf("%s should not expose right rail semantics below the wide threshold", tc.name)
				}
			}
		})
	}
}

func containsAll(value string, needles []string) bool {
	for _, needle := range needles {
		if !contains(value, needle) {
			return false
		}
	}
	return true
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if contains(value, needle) {
			return true
		}
	}
	return false
}

func contains(value string, needle string) bool {
	return strings.Contains(value, needle)
}

func TestRequiredM3RenderSnapshots(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"idle-empty", "narrow-80", "desktop-wide"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadStaticShellFixture(t, name)
			for _, tc := range fixture.TextCases() {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					got := tc.render(fixture.State, tc.size)
					assertTextSnapshot(t, fixture, tc.file, got)
				})
			}
		})
	}
}

func TestRequiredM3SemanticSnapshots(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"idle-empty", "narrow-80", "desktop-wide"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadStaticShellFixture(t, name)
			for _, tc := range fixture.SemanticCases() {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					got := RenderSemanticJSON(fixture.State, tc.size)
					assertSemanticSnapshot(t, fixture, tc.file, got)

					var snapshot SemanticSnapshot
					if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
						t.Fatalf("unmarshal semantic snapshot: %v", err)
					}
					if snapshot.Session.Phase != "placeholder" || snapshot.Session.PhaseSource != "not_started" {
						t.Fatalf("phase = %q from %q, want placeholder from not_started", snapshot.Session.Phase, snapshot.Session.PhaseSource)
					}
					if snapshot.Session.WorkflowTransition {
						t.Fatal("placeholder phase must not imply workflow transition behavior")
					}
					if len(snapshot.Actions) != 1 || snapshot.Actions[0].Input != "q" {
						t.Fatalf("actions = %+v, want single q quit action", snapshot.Actions)
					}
				})
			}
		})
	}
}

func TestRequiredM3SemanticContractConsistency(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"idle-empty", "narrow-80", "desktop-wide"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadStaticShellFixture(t, name)
			for _, tc := range fixture.SemanticCases() {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					snapshot := readSemanticSnapshot(t, fixture, tc.file)
					assertSemanticContract(t, name, tc.size, snapshot)
				})
			}
		})
	}
}

func readSemanticSnapshot(t *testing.T, fixture renderFixture, file string) SemanticSnapshot {
	t.Helper()

	var snapshot SemanticSnapshot
	if err := json.Unmarshal(fixture.ReadFile(t, file), &snapshot); err != nil {
		t.Fatalf("unmarshal semantic snapshot %s: %v", fixture.SnapshotPath(file), err)
	}
	return snapshot
}

func assertSemanticContract(t *testing.T, scenario string, size Size, snapshot SemanticSnapshot) {
	t.Helper()

	if snapshot.Scenario != scenario {
		t.Fatalf("scenario = %q, want %q", snapshot.Scenario, scenario)
	}
	if snapshot.Screen.Width != size.Width || snapshot.Screen.Height != size.Height {
		t.Fatalf("screen = %dx%d, want %dx%d", snapshot.Screen.Width, snapshot.Screen.Height, size.Width, size.Height)
	}
	if snapshot.Screen.Focus == "" {
		t.Fatal("screen focus is empty")
	}
	wantLayout := layoutForSize(size)
	if snapshot.Layout.Class != wantLayout.Class || snapshot.Layout.RightRailVisible != wantLayout.RightRailVisible {
		t.Fatalf("layout = %+v, want class %q right rail %v", snapshot.Layout, wantLayout.Class, wantLayout.RightRailVisible)
	}
	if snapshot.Session.Phase != "placeholder" || snapshot.Session.PhaseSource != "not_started" {
		t.Fatalf("phase = %q from %q, want placeholder from not_started", snapshot.Session.Phase, snapshot.Session.PhaseSource)
	}
	if snapshot.Session.WorkflowTransition || snapshot.Session.Active || snapshot.Session.QueuedMessages != 0 {
		t.Fatalf("placeholder session implies runtime workflow behavior: %+v", snapshot.Session)
	}
	if snapshot.Session.Autonomy == "" {
		t.Fatal("session autonomy is empty")
	}

	regions := map[string]SemanticRegion{}
	for _, region := range snapshot.Regions {
		if region.Name == "" {
			t.Fatal("semantic region name is empty")
		}
		if _, exists := regions[region.Name]; exists {
			t.Fatalf("duplicate semantic region %q", region.Name)
		}
		if !region.Visible {
			t.Fatalf("semantic region %q is hidden in the static shell contract", region.Name)
		}
		if len(region.Items) == 0 {
			t.Fatalf("semantic region %q has no agent-readable items", region.Name)
		}
		regions[region.Name] = region
	}
	for _, name := range []string{"header", "phase", "model", "chat", "prompt", "footer"} {
		if _, ok := regions[name]; !ok {
			t.Fatalf("semantic region %q missing from snapshot", name)
		}
	}
	if snapshot.Layout.RightRailVisible {
		if _, ok := regions["right_rail"]; !ok {
			t.Fatal("semantic right rail region missing when layout exposes the rail")
		}
	} else if _, ok := regions["right_rail"]; ok {
		t.Fatal("semantic right rail region present below the wide threshold")
	}
	if _, ok := regions[snapshot.Screen.Focus]; !ok {
		t.Fatalf("focus %q does not identify a visible semantic region", snapshot.Screen.Focus)
	}
	if !containsAll(strings.Join(regions["phase"].Items, "\n"), []string{"placeholder", "display-only"}) {
		t.Fatalf("phase region items = %v, want placeholder display-only semantics", regions["phase"].Items)
	}

	if len(snapshot.Actions) != 1 || snapshot.Actions[0].Name != "quit" || snapshot.Actions[0].Input != "q" {
		t.Fatalf("actions = %+v, want deterministic q quit action only", snapshot.Actions)
	}
}

func TestSnapshotUpdateModeIsExplicit(t *testing.T) {
	t.Setenv(updateSnapshotsEnv, "")
	if snapshotUpdateRequested(t) {
		t.Fatal("unset snapshot update mode should be read-only")
	}

	t.Setenv(updateSnapshotsEnv, "0")
	if snapshotUpdateRequested(t) {
		t.Fatal("disabled snapshot update mode should be read-only")
	}

	t.Setenv(updateSnapshotsEnv, "1")
	if !snapshotUpdateRequested(t) {
		t.Fatal("explicit snapshot update mode should be enabled")
	}
}

func TestStaticModelQuitPath(t *testing.T) {
	t.Parallel()

	model := NewModelWithSize(Size{Width: 80, Height: 24})
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	quitModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want tui.Model", updated)
	}
	if !quitModel.Quitting() {
		t.Fatal("q should mark the static model as quitting")
	}
	if cmd == nil {
		t.Fatal("q should emit a Bubble Tea quit command")
	}

	model = NewModelWithSize(Size{Width: 80, Height: 24})
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("Esc is out of scope for M2 and must not quit")
	}
	if updated.(Model).Quitting() {
		t.Fatal("Esc should not mark the static model as quitting")
	}
}
