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

func TestCommandSemanticsSurviveLayoutSizes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input string
		route string
	}{
		{name: "status-command", input: "/status", route: "status"},
		{name: "review-command", input: "/review", route: "review"},
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

func TestQueuedInputRenderShowsDefaultAfterCurrentTurn(t *testing.T) {
	t.Parallel()

	state := activeQueuedState()
	render := RenderPlain(state, Size{Width: 80, Height: 24})
	if !containsAll(render, []string{
		"Runtime active",
		"status: active",
		"active: true",
		"Queued input:",
		"queued messages: 2",
		"default action: send after current turn",
		"action status: presentation-only; not executed by the TUI",
		"queued: refine the tests",
		"queued: explain the diff",
		"user: active fake work",
	}) {
		t.Fatalf("queued render missing active work or queue defaults:\n%s", render)
	}
	if containsAny(render, []string{"interrupt", "steer", "cancel"}) {
		t.Fatalf("queued render implies non-default execution choices:\n%s", render)
	}
}

func TestQueuedInputSemanticDefaultAction(t *testing.T) {
	t.Parallel()

	snapshot := Semantic(activeQueuedState(), Size{Width: 80, Height: 24})
	if snapshot.Session.QueuedMessages != 2 {
		t.Fatalf("queued_messages = %d, want 2", snapshot.Session.QueuedMessages)
	}
	regions := semanticRegionsByName(t, snapshot)
	queue, ok := regions["queue"]
	if !ok || !queue.Visible {
		t.Fatalf("queue region = %+v, want visible queue region", queue)
	}
	if !containsAll(strings.Join(queue.Items, "\n"), []string{
		"queued messages: 2",
		"default action: send after current turn",
		"presentation-only",
		"executed: false",
		"queued: refine the tests",
		"queued: explain the diff",
	}) {
		t.Fatalf("queue semantic items = %+v", queue.Items)
	}

	var queueAction *SemanticAction
	for i := range snapshot.Actions {
		if snapshot.Actions[i].Name == "queue_after_current_turn" {
			queueAction = &snapshot.Actions[i]
		}
	}
	if queueAction == nil {
		t.Fatalf("actions = %+v, want queue_after_current_turn", snapshot.Actions)
	}
	if queueAction.Input != "enter" || !queueAction.Default || !queueAction.PresentationOnly || queueAction.Executed {
		t.Fatalf("queue action = %+v, want default presentation-only non-executed action", *queueAction)
	}
}

func TestNoQueueRenderAndSemanticRemainStable(t *testing.T) {
	t.Parallel()

	for _, state := range []ViewState{
		loadRuntimeStatusFixture(t, "runtime-idle").State,
		loadRuntimeStatusFixture(t, "runtime-active").State,
		loadRuntimeStatusFixture(t, "runtime-result").State,
	} {
		state := state
		t.Run(state.Scenario, func(t *testing.T) {
			t.Parallel()

			render := RenderPlain(state, Size{Width: 80, Height: 24})
			if containsAny(render, []string{"Queued input:", "default action: send after current turn", "queue_after_current_turn"}) {
				t.Fatalf("%s no-queue render gained queue UI:\n%s", state.Scenario, render)
			}

			snapshot := Semantic(state, Size{Width: 80, Height: 24})
			if snapshot.Session.QueuedMessages != 0 {
				t.Fatalf("%s queued_messages = %d, want 0", state.Scenario, snapshot.Session.QueuedMessages)
			}
			if _, ok := semanticRegionsByName(t, snapshot)["queue"]; ok {
				t.Fatalf("%s semantic regions include queue without queued input: %+v", state.Scenario, snapshot.Regions)
			}
			for _, action := range snapshot.Actions {
				if action.Name == "queue_after_current_turn" {
					t.Fatalf("%s actions include queue action without queued input: %+v", state.Scenario, snapshot.Actions)
				}
			}
		})
	}
}

func TestInterruptingRenderShowsActiveWorkAndInFlightState(t *testing.T) {
	t.Parallel()

	state := interruptState("canceling")
	render := RenderPlain(state, Size{Width: 80, Height: 24})

	if !containsAll(render, []string{
		"Runtime canceling",
		"status: canceling",
		"active: true",
		"interrupt state:",
		"interrupt status: canceling",
		"interrupt outcome: pending",
		"lower-layer cancellation executed: false",
		"user: active fake work",
	}) {
		t.Fatalf("interrupting render missing active work or in-flight state:\n%s", render)
	}
	if containsAny(render, []string{"shell canceled", "model canceled", "tool canceled", "runtime canceled by TUI"}) {
		t.Fatalf("interrupting render implies real lower-layer cancellation:\n%s", render)
	}
}

func TestCanceledRenderShowsActiveWorkAndFakeOutcome(t *testing.T) {
	t.Parallel()

	state := interruptState("canceled")
	render := RenderPlain(state, Size{Width: 80, Height: 24})

	if !containsAll(render, []string{
		"Runtime canceled",
		"status: canceled",
		"active: false",
		"result: fake work canceled",
		"interrupt state:",
		"interrupt status: canceled",
		"interrupt outcome: fake work canceled",
		"lower-layer cancellation executed: false",
		"user: active fake work",
	}) {
		t.Fatalf("canceled render missing active work or fake outcome:\n%s", render)
	}
	if containsAny(render, []string{"shell canceled", "model canceled", "tool canceled", "runtime canceled by TUI"}) {
		t.Fatalf("canceled render implies real lower-layer cancellation:\n%s", render)
	}
}

func TestInterruptSemanticIsMachineReadableAndNonExecuting(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		status  string
		active  bool
		outcome string
	}{
		{status: "canceling", active: true, outcome: "pending"},
		{status: "canceled", active: false, outcome: "fake work canceled"},
	} {
		tc := tc
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()

			snapshot := Semantic(interruptState(tc.status), Size{Width: 80, Height: 24})
			if snapshot.Interrupt == nil {
				t.Fatalf("%s semantic interrupt = nil", tc.status)
			}
			if snapshot.Interrupt.State != tc.status || snapshot.Interrupt.Outcome != tc.outcome || snapshot.Interrupt.LowerLayerCancellationExecuted {
				t.Fatalf("%s semantic interrupt = %+v", tc.status, *snapshot.Interrupt)
			}
			if snapshot.Session.RuntimeStatus != tc.status || snapshot.Session.Active != tc.active {
				t.Fatalf("%s semantic session = %+v", tc.status, snapshot.Session)
			}
			regions := semanticRegionsByName(t, snapshot)
			interrupt, ok := regions["interrupt"]
			if !ok || !interrupt.Visible {
				t.Fatalf("%s interrupt region = %+v", tc.status, interrupt)
			}
			if !containsAll(strings.Join(interrupt.Items, "\n"), []string{
				"state: " + tc.status,
				"outcome: " + tc.outcome,
				"lower_layer_cancellation_executed: false",
				"display-only",
			}) {
				t.Fatalf("%s interrupt semantic items = %+v", tc.status, interrupt.Items)
			}
		})
	}
}

func activeQueuedState() ViewState {
	state := IdleEmptyState()
	state.Scenario = "queued-message"
	state.Phase = "PLAN"
	state.PhaseSource = "workflow.fixture"
	state.RuntimeStatus = "active"
	state.StatusSource = "runtime.fixture"
	state.StatusDetail = "fake in-memory runtime loop"
	state.RuntimeActive = true
	state.QueuedCount = 2
	state.QueuedText = []string{"refine the tests", "explain the diff"}
	state.Transcript = []TranscriptTurn{{UserText: "active fake work"}}
	return state
}

func interruptState(status string) ViewState {
	state := IdleEmptyState()
	state.Scenario = "interrupt-" + status
	state.Phase = "BUILD"
	state.PhaseSource = "workflow.fixture"
	state.RuntimeStatus = status
	state.StatusSource = "runtime.fixture"
	state.StatusDetail = "fake in-memory runtime loop"
	state.RuntimeActive = status == "canceling"
	if status == "canceled" {
		state.RuntimeResult = "fake work canceled"
	}
	state.Transcript = []TranscriptTurn{{UserText: "active fake work"}}
	return state
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
		"status-command":   {"status:", "command route: status", "route source: policy.command", "app-owned status inspection unavailable in presentation-only fallback"},
		"review-command":   {"review:", "command route: review", "route source: policy.command", "app-owned review inspection unavailable in presentation-only fallback"},
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
		"status-command":   {"status", "command route: status", "route source: policy.command", "app-owned status inspection unavailable in presentation-only fallback"},
		"review-command":   {"review", "command route: review", "route source: policy.command", "app-owned review inspection unavailable in presentation-only fallback"},
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
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
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
