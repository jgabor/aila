package tui

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestM6ResponsiveRenderContract(t *testing.T) {
	t.Parallel()

	for _, fixture := range currentRenderFixtures(t) {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			plain80 := RenderPlain(fixture.State, Size{Width: 80, Height: 24})
			assertVisibleAt80(t, fixture.Name, plain80)
			assertActiveContentVisible(t, fixture.Name, plain80)
			if lines := strings.Count(plain80, "\n") + 1; lines > 24 {
				t.Fatalf("80x24 render uses %d rows, want at most 24:\n%s", lines, plain80)
			}

			for _, size := range []Size{{Width: 100, Height: 30}, {Width: 120, Height: 32}} {
				plain := RenderPlain(fixture.State, size)
				ansi := RenderANSI(fixture.State, size)
				statusToken := statusLine(fixture.State)
				if hasDisplayLabelDetails(fixture.State) {
					statusToken = "Stage " + fixture.State.Phase + " | Model " + fixture.State.PrimaryModel
				}
				if !containsAll(plain, []string{
					sizeString(size),
					statusToken,
					"Conversation",
				}) {
					t.Fatalf("%s render at %s lost deterministic layout structure:\n%s", fixture.Name, sizeString(size), plain)
				}
				if containsAny(plain, []string{"Session", "phase source:"}) || containsAny(ansi, []string{"Session", "phase source:"}) {
					t.Fatalf("%s render at %s introduced right rail below wide threshold:\n%s", fixture.Name, sizeString(size), plain)
				}
			}

			wide := RenderPlain(fixture.State, Size{Width: 160, Height: 45})
			assertActiveContentVisible(t, fixture.Name, wide)
			assertRightRailDisplayOnly(t, fixture.State, wide)
		})
	}
}

func TestM6SemanticLayoutContract(t *testing.T) {
	t.Parallel()

	for _, fixture := range currentRenderFixtures(t) {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			for _, size := range []Size{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 120, Height: 32}, {Width: 160, Height: 45}} {
				size := size
				t.Run(sizeString(size), func(t *testing.T) {
					t.Parallel()

					var snapshot SemanticSnapshot
					if err := json.Unmarshal([]byte(RenderSemanticJSON(fixture.State, size)), &snapshot); err != nil {
						t.Fatalf("unmarshal semantic JSON: %v", err)
					}
					assertSemanticContract(t, fixture.Name, size, snapshot)
					assertActiveSemanticContent(t, fixture.Name, snapshot)
				})
			}
		})
	}
}

func TestM6CommandSemanticsSurviveLayoutSizes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input string
		route string
	}{
		{name: "status-command", input: "/status", route: "status"},
		{name: "help-command", input: "/help", route: "help"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := loadCommandFixture(t, tc.name, tc.input)
			for _, size := range []Size{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 120, Height: 32}, {Width: 160, Height: 45}} {
				size := size
				t.Run(sizeString(size), func(t *testing.T) {
					t.Parallel()

					snapshot := Semantic(fixture.State, size)
					assertCommandSemanticContract(t, tc.name, size, tc.route, snapshot)
				})
			}
		})
	}
}

func currentRenderFixtures(t *testing.T) []renderFixture {
	t.Helper()
	return []renderFixture{
		loadStaticShellFixture(t, "idle-empty"),
		loadStaticShellFixture(t, "narrow-80"),
		loadStaticShellFixture(t, "desktop-wide"),
		loadDisplayFixture(t, "model-display"),
		loadDisplayFixture(t, "autonomy-display"),
		loadSubmittedPromptFixture(t),
		loadCommandFixture(t, "status-command", "/status"),
		loadCommandFixture(t, "help-command", "/help"),
	}
}

func assertVisibleAt80(t *testing.T, name string, render string) {
	t.Helper()
	state := loadFixtureStateForAssertion(name)
	statusToken := statusLine(state)
	if hasDisplayLabelDetails(state) {
		statusToken = "Stage " + state.Phase + " | Model " + state.PrimaryModel
	}
	for _, token := range []string{
		"Aila",
		statusToken,
		"Conversation",
		"Prompt",
		">",
		"git: placeholder | context: placeholder | q quit",
	} {
		if !contains(render, token) {
			t.Fatalf("%s 80x24 render missing %q:\n%s", name, token, render)
		}
	}
	if hasDisplayLabelDetails(state) && !containsAll(render, []string{"primary model: " + state.PrimaryModel, "utility model: " + state.UtilityModel, "autonomy: " + state.Autonomy + " (display-only)"}) {
		t.Fatalf("%s 80x24 render missing display label details:\n%s", name, render)
	}
}

func assertActiveContentVisible(t *testing.T, name string, render string) {
	t.Helper()
	want := map[string][]string{
		"idle-empty":       {"Conversation", "No messages yet."},
		"narrow-80":        {"Conversation", "No messages yet."},
		"desktop-wide":     {"Conversation", "No messages yet."},
		"model-display":    {"Conversation", "No messages yet.", "primary model: opencode-go/deepseek-v4-pro:high", "utility model: opencode-go/deepseek-v4-flash:max", "autonomy: yolo (display-only)"},
		"autonomy-display": {"Conversation", "No messages yet.", "primary model: opencode-go/deepseek-v4-pro:high", "utility model: opencode-go/deepseek-v4-flash:max", "autonomy: read (display-only)"},
		"submitted-prompt": {"user: explain this repo", "assistant: Fake Aila response: explain this repo"},
		"status-command":   {"status:", "command route: status", "route source: policy.command", "Deterministic placeholder status."},
		"help-command":     {"help:", "command route: help", "route source: policy.command", "Deterministic placeholder help."},
	}[name]
	if len(want) == 0 {
		t.Fatalf("test fixture %q has no active content assertion", name)
	}
	if !containsAll(render, want) {
		t.Fatalf("%s render missing active content %v:\n%s", name, want, render)
	}
}

func assertActiveSemanticContent(t *testing.T, name string, snapshot SemanticSnapshot) {
	t.Helper()

	regions := semanticRegionsByName(t, snapshot)
	joined := map[string]string{}
	for region, data := range regions {
		joined[region] = strings.Join(data.Items, "\n")
	}
	want := map[string][]string{
		"idle-empty":       {"No messages yet."},
		"narrow-80":        {"No messages yet."},
		"desktop-wide":     {"No messages yet."},
		"model-display":    {"No messages yet.", "primary: opencode-go/deepseek-v4-pro:high", "utility: opencode-go/deepseek-v4-flash:max", "autonomy: yolo"},
		"autonomy-display": {"No messages yet.", "primary: opencode-go/deepseek-v4-pro:high", "utility: opencode-go/deepseek-v4-flash:max", "autonomy: read"},
		"submitted-prompt": {"user: explain this repo", "assistant: Fake Aila response: explain this repo"},
		"status-command":   {"status", "command route: status", "route source: policy.command", "Deterministic placeholder status."},
		"help-command":     {"help", "command route: help", "route source: policy.command", "Deterministic placeholder help."},
	}[name]
	if len(want) == 0 {
		t.Fatalf("test fixture %q has no semantic active content assertion", name)
	}
	content := joined["chat"] + "\n" + joined["command"] + "\n" + joined["model"]
	if !containsAll(content, want) {
		t.Fatalf("%s semantic output missing active content %v in %+v", name, want, regions)
	}
}

func assertRightRailDisplayOnly(t *testing.T, state ViewState, render string) {
	t.Helper()
	railStart := strings.Index(render, "Session")
	if railStart < 0 {
		t.Fatalf("160x45 render missing Session rail:\n%s", render)
	}
	rail := render[railStart:]
	if !containsAll(rail, []string{
		"phase source: " + state.PhaseSource,
		"primary model: " + state.PrimaryModel,
		"utility model: " + state.UtilityModel,
		"autonomy: " + state.Autonomy,
		"git: " + state.FooterGit,
		"context: " + state.FooterContext,
	}) {
		t.Fatalf("right rail missing display/supporting labels:\n%s", rail)
	}
	if containsAny(rail, []string{"workflow", "provider", "permission"}) {
		t.Fatalf("right rail contains behavior or real lookup content:\n%s", rail)
	}
}

func loadFixtureStateForAssertion(name string) ViewState {
	state := IdleEmptyState()
	state.Scenario = name
	switch name {
	case "model-display":
		state.PrimaryModel = "opencode-go/deepseek-v4-pro:high"
		state.UtilityModel = "opencode-go/deepseek-v4-flash:max"
		state.Autonomy = "yolo"
	case "autonomy-display":
		state.PrimaryModel = "opencode-go/deepseek-v4-pro:high"
		state.UtilityModel = "opencode-go/deepseek-v4-flash:max"
		state.Autonomy = "read"
	}
	return state
}

func sizeString(size Size) string {
	return strconv.Itoa(size.Width) + "x" + strconv.Itoa(size.Height)
}
