package tui

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
)

const (
	testWorkflowPhaseLabel  = "IDLE"
	testWorkflowPhaseSource = "idle"
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
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.Scenario = name
	return loadRenderFixture(t, name, state)
}

func loadSubmittedPromptFixture(t *testing.T) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.Scenario = "submitted-prompt"
	state.Transcript = []TranscriptTurn{{
		UserText:      "explain this repo",
		AssistantText: "Fake Aila response: explain this repo",
	}}
	return loadRenderFixture(t, state.Scenario, state)
}

func loadSessionCommandFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	switch name {
	case "fresh-session":
		state = ApplySessionView(state, &SessionView{
			Action:       "new",
			Source:       "app.session",
			Status:       "fresh",
			SessionID:    "current",
			MemoryStatus: "fresh",
			Detail:       "started fresh session and preserved project store",
		})
	case "cleared-session":
		state = ApplySessionView(state, &SessionView{
			Action:       "clear",
			Source:       "app.session",
			Status:       "cleared",
			SessionID:    "current",
			MemoryStatus: "cleared",
			Detail:       "cleared visible session and current memory",
		})
	case "resumed-session":
		state.RuntimeStatus = "idle"
		state.StatusSource = "runtime.dispatch"
		state.StatusDetail = "resumed current session"
		state.RuntimeResult = "remembered result"
		state.MemorySource = "state.current-session-snapshot"
		state.MemorySessionID = "current"
		state.Transcript = []TranscriptTurn{
			{UserText: "remembered prompt"},
			{AssistantText: "remembered answer"},
		}
		state = ApplySessionView(state, &SessionView{
			Action:       "continue",
			Source:       "app.session",
			Status:       "loaded",
			SessionID:    "current",
			MemoryStatus: "visible",
			Detail:       "restored current session snapshot",
			Focus:        true,
			Items: []SessionItemView{{
				ID:           "current",
				Status:       "loaded",
				MemoryStatus: "visible",
				Detail:       "current session",
			}},
		})
	default:
		t.Fatalf("unknown session command fixture %q", name)
	}
	state.Scenario = name
	return loadRenderFixture(t, name, state)
}

func loadCommandFixture(t *testing.T, name string, input string) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	model := NewModelWithStateSizePromptSubmitAndCommandRoute(state, Size{Width: 80, Height: 24}, nil, nil)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(input)})
	if cmd != nil {
		t.Fatalf("typing %s emitted a command", input)
	}
	updated, cmd = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("routing %s emitted a command", input)
	}
	state = updated.(Model).state
	state.Scenario = name
	return loadRenderFixture(t, name, state)
}

func loadCompactFixture(t *testing.T, name string) renderFixture {
	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.Scenario = name
	switch name {
	case "compact-running":
		state.RuntimeStatus = "idle"
		state.StatusSource = "runtime.dispatch"
		state.StatusDetail = "background context compaction"
		state.RuntimeActive = false
		state.Compact = &CompactView{
			Mode:    "background",
			Source:  "app.compact.background",
			Status:  "running",
			Summary: "background context compaction running",
		}
	case "compact-result":
		refs := []ContextSourceRefView{
			{ID: "command-1", Kind: "command", Command: "git status --short", Excerpt: "git status --short"},
			{ID: "command-1-stdout-1", Kind: "command_stdout", Command: "git status --short", Stream: "stdout", Excerpt: "M internal/context/context.go"},
		}
		state.RuntimeStatus = "idle"
		state.StatusSource = "runtime.dispatch"
		state.StatusDetail = "manual context compaction"
		state.RuntimeResult = "manual compaction preserved 2 source refs (1 caveat)"
		state.Compact = &CompactView{
			Mode:          "manual",
			Source:        "app.compact",
			Status:        "flagged",
			Summary:       "manual compaction preserved 2 source refs",
			OriginalMeter: "1 blocks / 2 refs / 128 bytes of 4096",
			Meter:         "1 blocks / 2 refs / 256 bytes of 4096",
			Caveats:       []string{"input context warning: shell stdout truncated"},
			SourceRefs:    refs,
		}
		state.Context = &ContextView{
			Source: "app.compact",
			Status: "compacted",
			Meter:  "1 blocks / 2 refs / 256 bytes of 4096",
			Blocks: []ContextBlockView{{
				ID:           "compacted-context",
				Kind:         "compacted_context",
				Title:        "Manual compacted context",
				Text:         "compacted 1 blocks, 1 claims, and 2 source refs\nsource command-1-stdout-1 command_stdout git status --short stdout: M internal/context/context.go",
				SourceRefIDs: []string{"command-1", "command-1-stdout-1"},
			}},
			Claims:     []ContextClaimView{{Text: "manual compaction preserved 2 source refs", SourceRefIDs: []string{"command-1", "command-1-stdout-1"}}},
			SourceRefs: refs,
			Warnings:   []string{"input context warning: shell stdout truncated"},
		}
	default:
		t.Fatalf("unknown compact fixture %q", name)
	}
	return loadRenderFixture(t, name, state)
}

func loadUtilityFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.Scenario = name
	state.RuntimeStatus = "idle"
	state.StatusSource = "runtime.fixture"
	state.StatusDetail = "utility worker status"
	state.UtilityModel = "opencode-go/deepseek-v4-flash:max"
	state.Utility = &UtilityView{
		Source:   "app.status",
		Status:   "idle",
		JobID:    "status-utility-suggestion",
		JobKind:  "suggestion",
		Model:    state.UtilityModel,
		Summary:  "idle utility worker ready",
		ReadOnly: true,
	}
	switch name {
	case "utility-idle":
	case "utility-running":
		state.Utility.Status = "running"
		state.Utility.Summary = "fake utility job running"
	case "utility-result":
		state.Utility.Status = "completed"
		state.Utility.Summary = "fake utility suggestion ready"
		state.Utility.Suggestions = []UtilitySuggestionView{{Text: "Review current status before starting new background utility work.", EvidenceRefIDs: []string{"utility-evidence-1"}}}
		state.Utility.EvidenceRefs = []UtilityEvidenceRefView{{ID: "utility-evidence-1", Kind: "runtime_state", Source: "app.status", Detail: "primary runtime idle; fake utility job only"}}
	case "utility-context-prep":
		state.Utility.Status = "completed"
		state.Utility.JobID = "status-context-prep"
		state.Utility.JobKind = "context_prep"
		state.Utility.Summary = "prepared context ready"
		state.Utility.PreparedContext = UtilityPreparedContextView{
			Summary:          "Likely next context: roadmap M42 scope, current utility worker state, and recent status evidence.",
			EvidenceRefIDs:   []string{"context-prep-roadmap", "context-prep-runtime"},
			Caveats:          []string{"prepared context is non-authoritative; foreground work must re-check source refs before acting"},
			NonAuthoritative: true,
		}
		state.Utility.Suggestions = []UtilitySuggestionView{{Text: "Use prepared context only as a starting point for the next foreground step.", EvidenceRefIDs: []string{"context-prep-roadmap", "context-prep-runtime"}}}
		state.Utility.EvidenceRefs = []UtilityEvidenceRefView{
			{ID: "context-prep-roadmap", Kind: "roadmap", Source: "ROADMAP.md", Detail: "Milestone 42 requires visible non-authoritative utility context prep"},
			{ID: "context-prep-runtime", Kind: "runtime_state", Source: "app.status", Detail: "primary runtime idle; context prep allowed by utility scheduler"},
		}
		state.Utility.Caveats = []string{"prepared context is non-authoritative; foreground capability decides whether to use it"}
	case "utility-stale-context":
		state.Utility.Status = "completed"
		state.Utility.JobID = "status-stale-context-check"
		state.Utility.JobKind = "stale_context_check"
		state.Utility.Summary = "saved context appears stale"
		state.Utility.StaleContext = UtilityStaleContextView{
			Status:              "stale",
			Summary:             "saved context appears stale",
			EvidenceRefIDs:      []string{"stale-context-saved", "stale-context-current"},
			Caveats:             []string{"stale status is advisory; no context was refreshed, compacted, or rewritten"},
			SuggestedNextAction: "Rebuild foreground context before relying on saved context.",
		}
		state.Utility.Suggestions = []UtilitySuggestionView{{Text: "Rebuild foreground context before relying on saved context.", EvidenceRefIDs: []string{"stale-context-saved", "stale-context-current"}}}
		state.Utility.EvidenceRefs = []UtilityEvidenceRefView{
			{ID: "stale-context-saved", Kind: "context_fingerprint", Source: "saved context", Detail: "saved=saved-context:utility-context-prep"},
			{ID: "stale-context-current", Kind: "context_fingerprint", Source: "current runtime status", Detail: "current=current-context:status-runtime"},
		}
		state.Utility.Caveats = []string{"stale status is advisory; no context was refreshed, compacted, or rewritten"}
	case "utility-summary-refresh":
		state.Utility.Status = "completed"
		state.Utility.JobID = "status-summary-refresh"
		state.Utility.JobKind = "summary_refresh"
		state.Utility.Summary = "summary refreshed with source-backed details"
		state.Utility.SummaryRefresh = UtilitySummaryRefreshView{
			Status:           "refreshed",
			OriginalSummary:  "Runtime summary mentions status only.",
			RefreshedSummary: "Runtime summary mentions status only. Important details: primary runtime remains idle; source refs stay visible",
			SourceRefIDs:     []string{"summary-refresh-runtime", "summary-refresh-roadmap"},
			ExactDetails:     []string{"primary runtime remains idle", "source refs stay visible"},
			Confidence:       "high",
		}
		state.Utility.Suggestions = []UtilitySuggestionView{{Text: "Use the refreshed summary only with its preserved source refs.", EvidenceRefIDs: []string{"summary-refresh-source-1", "summary-refresh-source-2", "summary-refresh-detail-1", "summary-refresh-detail-2"}}}
		state.Utility.EvidenceRefs = []UtilityEvidenceRefView{
			{ID: "summary-refresh-source-1", Kind: "source_ref", Source: "app.status", Detail: "source_ref=summary-refresh-runtime"},
			{ID: "summary-refresh-source-2", Kind: "source_ref", Source: "app.status", Detail: "source_ref=summary-refresh-roadmap"},
			{ID: "summary-refresh-detail-1", Kind: "exact_detail", Source: "app.status", Detail: "primary runtime remains idle"},
			{ID: "summary-refresh-detail-2", Kind: "exact_detail", Source: "app.status", Detail: "source refs stay visible"},
		}
	default:
		t.Fatalf("unknown utility fixture %q", name)
	}
	return loadRenderFixture(t, name, state)
}

func loadPolicyRoutingFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.Scenario = name
	state.RuntimeStatus = "idle"
	state.StatusSource = "runtime.fixture"
	state.StatusDetail = "policy routing evidence"
	state.UtilityModel = "opencode-go/deepseek-v4-flash:max"
	route := &PolicyRouteView{
		Source:            "policy.capability",
		CurrentPhase:      "deliberate",
		TransitionClaimed: false,
		Executed:          false,
		SourceRefs: []PolicyRouteSourceRefView{{
			ID:   "policy-route-source",
			Kind: "prompt",
		}},
	}
	switch name {
	case "policy-explicit-route":
		route.Source = "policy.capability.explicit_slash"
		route.Input = "/plan"
		route.Candidate = "plan"
		route.Confidence = 100
		route.Reason = "exact capability slash route"
		route.RecommendedSuccessor = "plan"
		route.SuccessorValid = true
		route.SuccessorReason = "workflow FSM accepted recommended successor"
		route.SourceRefs[0].ID = "policy-explicit-route"
		route.SourceRefs[0].Excerpt = "/plan"
		route.BoundaryRequests = []PolicyRouteBoundaryRequestView{
			{Kind: "model_call", Operation: "model.call", Target: "primary model", Reason: "runtime owns model loop"},
			{Kind: "artifact_access", Operation: "artifact.access", Target: "plan artifact", Reason: "state resolver owns artifact access"},
		}
	case "policy-natural-language-route":
		route.Source = "policy.capability.natural_language"
		route.Input = "please audit the boundary risks"
		route.Candidate = "audit"
		route.Confidence = 86
		route.Reason = "audit intent matched"
		route.CurrentPhase = "build"
		route.RecommendedSuccessor = "audit"
		route.SuccessorValid = true
		route.SuccessorReason = "workflow FSM accepted recommended successor"
		route.SourceRefs[0].ID = "policy-natural-language-route"
		route.SourceRefs[0].Excerpt = "please audit the boundary risks"
	case "policy-waiting-route":
		route.Source = "policy.capability.waiting"
		route.Input = "help"
		route.Candidate = "brief"
		route.Confidence = 42
		route.Reason = "low confidence capability route"
		route.NeededInput = "Clarify whether you want a brief, discussion, plan, build, or audit."
		route.CurrentPhase = "plan"
		route.RuntimeStatus = "waiting"
		route.SourceRefs[0].ID = "policy-low-confidence-route"
		route.SourceRefs[0].Excerpt = "help"
	case "policy-invalid-successor":
		route.Source = "policy.capability.successor_validation"
		route.Input = "capability exit recommended deliberate"
		route.Candidate = "build"
		route.Confidence = 100
		route.Reason = "capability exit successor requires workflow FSM validation"
		route.CurrentPhase = "build"
		route.RecommendedSuccessor = "deliberate"
		route.SuccessorRejected = true
		route.SuccessorReason = "invalid workflow successor from \"build\" to \"deliberate\": invalid_edge"
		route.SourceRefs[0].ID = "policy-invalid-successor"
		route.SourceRefs[0].Kind = "capability_exit"
		route.SourceRefs[0].Excerpt = "build cannot jump back to deliberate"
		route.BoundaryRequests = []PolicyRouteBoundaryRequestView{{Kind: "permission_check", Operation: "tool.write", Target: "ROADMAP.md", Reason: "permission owns approval"}}
	default:
		t.Fatalf("unknown policy routing fixture %q", name)
	}
	state = ApplyPolicyRouteView(state, route)
	return loadRenderFixture(t, name, state)
}

func policyRoutingFixtureSizes() []fixtureSize {
	return []fixtureSize{{Name: "120x44", Width: 120, Height: 44}}
}

func utilityFixtureSizes() []fixtureSize {
	return []fixtureSize{{Name: "120x44", Width: 120, Height: 44}}
}

func loadDisplayFixture(t *testing.T, name string) renderFixture {
	t.Helper()

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
	default:
		t.Fatalf("unknown display fixture %q", name)
	}
	return loadRenderFixture(t, name, state)
}

func loadSwitchFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.PrimaryModel = "opencode-go/deepseek-v4-pro:high"
	state.UtilityModel = "opencode-go/deepseek-v4-flash:max"
	state.Autonomy = "yolo"
	switch name {
	case "model-switch":
		state = ApplyModelSwitchView(state, &ModelSwitchView{
			Target:         string(policy.CommandTargetPrimaryModel),
			Source:         "app.model",
			Status:         "ready",
			CurrentPrimary: state.PrimaryModel,
			CurrentUtility: state.UtilityModel,
			Detail:         "session-scoped model selection; config file unchanged",
			Focus:          true,
			Items: []ModelSwitchItemView{
				{Label: "opencode-go/deepseek-v4-pro:high", SourceName: "opencode-go", Model: "deepseek-v4-pro", Reasoning: "high", Family: "device_code", Class: "reasoning", Status: "available", CredentialSource: "device-code", Detail: "current session primary model", Current: true},
				{Label: "openai/gpt-4.1", SourceName: "openai", Model: "gpt-4.1", Family: "api_key", Class: "general", Status: "degraded", CredentialSource: "OPENAI_API_KEY", Detail: "readiness timeout"},
				{Label: "custom/deepseek-chat", SourceName: "custom", Model: "deepseek-chat", Family: "custom", Class: "general", Status: "unavailable", CredentialSource: "OPENAI_API_KEY", Detail: "provider unavailable"},
				{Label: "opencode-go/deepseek-v4-flash", SourceName: "opencode-go", Model: "deepseek-v4-flash", Family: "device_code", Class: "utility", Status: "available", CredentialSource: "device-code", Detail: "deterministic readiness row"},
			},
		})
	case "autonomy-switch":
		state.Autonomy = "read"
		state = ApplyAutonomySwitchView(state, &AutonomySwitchView{
			Source:   "app.autonomy",
			Status:   "ready",
			Current:  "read",
			Detail:   "session-scoped autonomy selection; config file unchanged",
			Selected: 1,
			Focus:    true,
			Items: []AutonomySwitchItemView{
				{Level: "off", Status: "available", Detail: "approval required before read or write operations"},
				{Level: "read", Status: "available", Detail: "read-only operations may run automatically", Current: true},
				{Level: "write", Status: "available", Detail: "workspace write operations may run automatically"},
				{Level: "yolo", Status: "available", Detail: "highest autonomy for classified operations"},
			},
		})
	default:
		t.Fatalf("unknown switch fixture %q", name)
	}
	state.Scenario = name
	return loadRenderFixture(t, name, state)
}

func loadPromptInputFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	switch name {
	case "editor-open":
		state = ApplyPromptInputText(state, "draft prompt")
		state = ApplyPromptEditorView(state, &PromptEditorView{Source: "app.editor", Status: "open", Detail: "editing current prompt through app-owned editor runner"})
	case "file-reference-picker":
		state = ApplyPromptInputText(state, "summarize @")
		state = ApplyFileReferenceView(state, &FileReferenceView{
			Source: "app.file-reference",
			Status: "ready",
			Query:  "",
			Detail: "3 files",
			Focus:  true,
			Items: []FileReferenceItemView{
				{Path: "README.md", Detail: "read-only discovery"},
				{Path: "docs/guide.md", Detail: "read-only discovery"},
				{Path: "internal/app.go", Detail: "read-only discovery"},
			},
		})
	case "file-reference-inserted":
		state = ApplyPromptInputText(state, "summarize @docs/guide.md")
		state = ApplyFileReferenceView(state, &FileReferenceView{
			Source:   "app.file-reference",
			Status:   "inserted",
			Detail:   "inserted docs/guide.md",
			Selected: 1,
			Items: []FileReferenceItemView{
				{Path: "README.md", Detail: "read-only discovery"},
				{Path: "docs/guide.md", Detail: "read-only discovery"},
			},
		})
	case "pasted-lines":
		state = ApplyPromptInputText(state, "alpha\nbeta\ngamma\ndelta")
	default:
		t.Fatalf("unknown prompt input fixture %q", name)
	}
	state.Scenario = name
	return loadRenderFixture(t, name, state)
}

func loadProjectStoreFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.Scenario = name
	state.PrimaryModel = "opencode-go/deepseek-v4-pro:high"
	state.UtilityModel = "opencode-go/deepseek-v4-flash:max"
	state.Autonomy = "read"
	state.ProjectStoreSource = "state.open"
	switch name {
	case "store-initialized":
		state.ProjectStoreStatus = "initialized"
		state.ProjectStoreDetail = "project store ready"
	case "store-uninitialized":
		state.ProjectStoreStatus = "uninitialized"
		state.ProjectStoreDetail = "project store not opened"
	case "store-degraded":
		state.ProjectStoreStatus = "degraded"
		state.ProjectStoreDetail = "create store directory"
	default:
		t.Fatalf("unknown project store fixture %q", name)
	}
	return loadRenderFixture(t, name, state)
}

func loadDiagnosticFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.Scenario = name
	switch name {
	case "diagnostic-ready":
		state.Diagnostics = []DiagnosticView{{
			Severity:         "warning",
			Source:           "runtime.fixture",
			RecoveryAction:   "inspect",
			AffectedArtifact: "runtime",
			UserInputNeeded:  false,
			BoundedMessage:   "runtime cancellation was recorded as diagnostic state",
		}}
	case "corrupt-state-recovery":
		state.ProjectStoreStatus = "recovery-needed"
		state.ProjectStoreSource = "state.open"
		state.ProjectStoreDetail = "project metadata needs manual review"
		state.Diagnostics = []DiagnosticView{{
			Severity:         "error",
			Source:           "state.open",
			RecoveryAction:   "manual-repair",
			AffectedArtifact: "project-metadata",
			UserInputNeeded:  true,
			BoundedMessage:   "metadata unreadable; inspect before reinitialize",
		}}
	case "graceful-shutdown":
		state.RuntimeStatus = "canceled"
		state.StatusSource = "signal.fixture"
		state.StatusDetail = "shutdown completed without repair"
		state.RuntimeResult = "cancellation recorded"
		state.Diagnostics = []DiagnosticView{{
			Severity:         "info",
			Source:           "signal.shutdown",
			RecoveryAction:   "none",
			AffectedArtifact: "runtime",
			UserInputNeeded:  false,
			BoundedMessage:   "graceful shutdown completed after cancellation",
		}}
	default:
		t.Fatalf("unknown diagnostic fixture %q", name)
	}
	return loadRenderFixture(t, name, state)
}

func loadWaitingStatusFixture(t *testing.T) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = "PLAN"
	state.PhaseSource = "workflow.fixture"
	state.RuntimeStatus = "waiting"
	state.StatusSource = "runtime.fixture"
	state.StatusDetail = "successor blocked by injected blocker"
	state.Scenario = "waiting-transition"
	return loadRenderFixture(t, state.Scenario, state)
}

func loadRuntimeStatusFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	base := runtime.Model{Status: runtime.StatusIdle}
	var model runtime.Model
	switch name {
	case "runtime-idle":
		model = base
	case "runtime-active":
		var effects []runtime.Effect
		model, effects = runtime.Update(base, runtime.PromptSubmitted{Text: "explain runtime status"})
		if len(effects) != 1 {
			t.Fatalf("active fixture effects = %d, want one pending fake effect", len(effects))
		}
	case "runtime-result":
		var effects []runtime.Effect
		model, effects = runtime.Update(base, runtime.PromptSubmitted{Text: "explain runtime status"})
		for _, message := range runtime.Dispatch(effects) {
			model, _ = runtime.Update(model, message)
		}
	default:
		t.Fatalf("unknown runtime status fixture %q", name)
	}

	state := viewStateFromRuntimeModel(name, model)
	return loadRenderFixture(t, name, state)
}

func loadQueuedMessageFixture(t *testing.T) renderFixture {
	t.Helper()

	return loadRenderFixture(t, "queued-message", activeQueuedState())
}

func loadIdleWithMemoryFixture(t *testing.T) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.Scenario = "idle-with-memory"
	state.RuntimeStatus = "idle tok\x1b[31men=secret-value"
	state.StatusSource = "runtime.dispatch api_\x1b[31mkey=secret-value"
	state.StatusDetail = "resumed current session pass\x1b[0mword=secret-value"
	state.RuntimeResult = "remembered result Bear\x1b[31mer secret-token"
	state.MemorySource = "state.current-session-snapshot"
	state.MemorySessionID = "current"
	state.Transcript = []TranscriptTurn{
		{UserText: "remembered prompt tok\x1b[31men=secret-value"},
		{AssistantText: "remembered answer with Bear\x1b[31mer secret-token"},
	}
	state.QueuedCount = 2
	state.QueuedText = []string{"queued follow-up", "queued api_\x1b[31mkey=secret-value"}
	state.MemoryBlockers = []string{"interrupt pending", "blocked by pass\x1b[31mword=secret-value"}
	state.MemoryConcerns = []string{"phase=IDLE runtime=idle", "concern with se\x1b[2mcret=hidden"}
	state.Diagnostics = []DiagnosticView{{
		Severity:         "warning",
		Source:           "state.snapshot",
		RecoveryAction:   "inspect",
		AffectedArtifact: "session-snapshot",
		UserInputNeeded:  false,
		BoundedMessage:   "remembered diagnostic tok\x1b[31men=secret-value",
	}}
	return loadRenderFixture(t, state.Scenario, state)
}

func loadHistoryViewFixture(t *testing.T) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.Scenario = "history-view"
	items := []HistoryItem{
		{
			EventID:     "evt-fake-001",
			RunID:       "run-fake-017",
			SessionID:   "session-fake-alpha",
			Kind:        "prompt",
			Source:      "user",
			Provenance:  "prompt.submit",
			DisplayText: "prompt summary: inspect fake history",
		},
		{
			EventID:     "evt-fake-002",
			RunID:       "run-fake-017",
			SessionID:   "session-fake-alpha",
			Kind:        "response",
			Source:      "runtime.fake",
			Provenance:  "model.response",
			DisplayText: "response summary: fake answer for history",
		},
		{
			EventID:     "evt-fake-003",
			RunID:       "run-fake-017",
			SessionID:   "session-fake-alpha",
			Kind:        "command",
			Source:      "policy.command",
			Provenance:  "slash.route",
			DisplayText: "command summary: /status rendered only",
		},
		{
			EventID:     "evt-fake-004",
			RunID:       "run-fake-017",
			SessionID:   "session-fake-alpha",
			Kind:        "runtime",
			Source:      "runtime.fixture",
			Provenance:  "runtime.update",
			DisplayText: "runtime summary: idle after fake event",
		},
		{
			EventID:     "evt-fake-005",
			RunID:       "run-fake-017",
			SessionID:   "session-fake-alpha",
			Kind:        "diagnostic",
			Source:      "state.fixture",
			Provenance:  "redaction.fixture",
			DisplayText: "credential token=abc123 path $HOME/git/aila/.aila/project.toml \x1b[31mcontrol",
		},
		{
			EventID:     "evt-fake-006",
			RunID:       "run-fake-017",
			SessionID:   "session-fake-alpha",
			Kind:        "mutation",
			Source:      "mutation.tool",
			Provenance:  "mutation.result",
			DisplayText: "mutation write completed notes.txt approval fake-approval-write-001 undo delete_created_file",
			Mutation: &HistoryMutationItem{
				Name:                  "write",
				Status:                "completed",
				CommandSource:         "approval-write",
				RequestID:             "fake-approval-write",
				ApprovalID:            "fake-approval-write-001",
				ApprovalAction:        "approve",
				ChangedPaths:          []string{"notes.txt"},
				RequestedPath:         "notes.txt",
				ExpectedEffect:        "create fake approval write target through explicit mutation effect",
				PreviousVersion:       "missing",
				NewVersion:            "sha256:fixture-new-version",
				BytesWritten:          23,
				ResolvedPathAvailable: true,
				DecisionRunID:         "op-write-fixture",
				DecisionCapability:    "approval-write",
			},
			Undo: &HistoryUndoItem{
				Available:       true,
				Action:          "delete_created_file",
				Paths:           []string{"notes.txt"},
				PreviousVersion: "missing",
				NewVersion:      "sha256:fixture-new-version",
			},
		},
		{
			EventID:     "evt-fake-007",
			RunID:       "run-fake-017",
			SessionID:   "session-fake-alpha",
			Kind:        "recovery",
			Source:      "recovery.command",
			Provenance:  "recovery.undo",
			DisplayText: "recovery undo completed notes.txt target evt-fake-006 redo restore_created_file",
			Recovery: &HistoryRecoveryItem{
				Command:            "undo",
				Status:             "completed",
				TargetEventID:      "evt-fake-006",
				Action:             "delete_created_file",
				Paths:              []string{"notes.txt"},
				PreviousVersion:    "sha256:fixture-new-version",
				NewVersion:         "missing",
				RedoAvailable:      true,
				RedoAction:         "restore_created_file",
				DecisionRunID:      "current",
				DecisionCapability: "recovery.undo",
			},
		},
		{
			EventID:     "evt-fake-008",
			RunID:       "run-fake-017",
			SessionID:   "session-fake-alpha",
			Kind:        "recovery",
			Source:      "recovery.command",
			Provenance:  "recovery.redo",
			DisplayText: "recovery redo completed notes.txt target evt-fake-006",
			Recovery: &HistoryRecoveryItem{
				Command:            "redo",
				Status:             "completed",
				TargetEventID:      "evt-fake-006",
				Action:             "restore_created_file",
				Paths:              []string{"notes.txt"},
				PreviousVersion:    "missing",
				NewVersion:         "sha256:fixture-redone-version",
				RedoAvailable:      false,
				DecisionRunID:      "current",
				DecisionCapability: "recovery.redo",
			},
		},
	}
	state = ApplyHistoryView(state, items, 7, true)
	return loadRenderFixture(t, state.Scenario, state)
}

func loadDiffViewFixture(t *testing.T) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.Scenario = "diff-view"
	state = ApplyDiffView(state, &DiffView{
		Source: "app.diff.fixture",
		Status: "ready",
		Files: []DiffFileView{{
			Path:    "internal/demo.txt",
			OldPath: "internal/demo.txt",
			Status:  "modified",
			Hunks: []DiffHunkView{{
				Header:   "@@ -1 +1 @@",
				OldStart: 1,
				OldLines: 1,
				NewStart: 1,
				NewLines: 1,
				Lines: []DiffLineView{
					{Kind: "removal", Text: "old value", OldLine: 1},
					{Kind: "addition", Text: "new value", NewLine: 1},
				},
			}},
		}},
	}, 3, true)
	return loadRenderFixture(t, state.Scenario, state)
}

func diffViewFixtureSizes() []fixtureSize {
	return []fixtureSize{{Name: "80x24", Width: 80, Height: 24}, {Name: "120x32", Width: 120, Height: 32}, {Name: "160x45", Width: 160, Height: 45}}
}

func TestSafeTextStripsTerminalControlsBeforeRedactingSecrets(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"tok\x1b[31men=secret-value",
		"api_\x1b[31mkey=secret-value",
		"pass\x1b[31mword=secret-value",
		"se\x1b[31mcret=secret-value",
		"Bear\x1b[31mer secret-token",
	} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			got := safeText(input)
			if got != "[redacted]" {
				t.Fatalf("safeText(%q) = %q, want [redacted]", input, got)
			}
			assertNoMemoryLeak(t, got)
		})
	}
}

func TestSafeTextStripsTerminalControlPayloads(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"title \x1b]0;token=secret-value\a after",
		"payload \x1bPpassword=secret-value\x1b\\ after",
		"payload \x1b_secret=hidden\x1b\\ after",
		"title \x9dtoken=secret-value\a after",
		"payload \x90password=secret-value\x1b\\ after",
		"payload \x9esecret=hidden\x1b\\ after",
		"payload \x9fsecret=hidden\x1b\\ after",
	} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			got := safeText(input)
			assertNoReadLeak(t, got)
			if containsAny(got, []string{"token", "password", "hidden"}) {
				t.Fatalf("safeText(%q) = %q, want terminal control payload stripped", input, got)
			}
		})
	}
}

func TestSafeTextRedactsPathLikeText(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"workspace /home/jgabor/git/aila/internal/tui",
		"system /etc/passwd",
		"logs /var/log/auth.log",
		"home $HOME/.ssh/id_rsa",
		"config ${XDG_CONFIG_HOME}/aila/config.toml",
		"store /home/jgabor/git/aila/.aila/project.toml",
		"config ~/.config/aila/config.toml",
		"scratch /tmp/aila/artifacts/indexes/cache",
		`windows C:\Users\jgabor\AppData\Roaming\aila\config.toml`,
	} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			got := safeText(input)
			if !contains(got, "[path-redacted]") {
				t.Fatalf("safeText(%q) = %q, want path redaction", input, got)
			}
			assertNoPathLeak(t, got)
		})
	}
}

func TestM18ReadToolRunningRenderAndSemantic(t *testing.T) {
	t.Parallel()

	state := readToolState(&ReadView{
		Name:           "read",
		Status:         "running",
		ReadOnly:       true,
		Path:           "internal/tui/read_result.go",
		RequestedRange: ReadLineRangeView{StartLine: 12, Limit: 8},
	})
	render := RenderPlain(state, Size{Width: 100, Height: 30})
	if !containsAll(render, []string{
		"Read tool:",
		"tool: read",
		"status: running",
		"read-only: true",
		"path: internal/tui/read_result.go",
		"requested range: start 12 limit 8",
		"completed: false",
	}) {
		t.Fatalf("running read render missing tool-running evidence:\n%s", render)
	}
	if containsAny(render, []string{"effective range:", "preview:", "error kind:", "completed: true"}) {
		t.Fatalf("running read render looks completed:\n%s", render)
	}

	snapshot := Semantic(state, Size{Width: 100, Height: 30})
	if snapshot.Read == nil || snapshot.Read.Name != "read" || snapshot.Read.Status != "running" || !snapshot.Read.ReadOnly || snapshot.Read.Path != "internal/tui/read_result.go" || snapshot.Read.Completed {
		t.Fatalf("running read semantic = %+v, want read-only running metadata", snapshot.Read)
	}
	if snapshot.Read.RequestedRange.StartLine != 12 || snapshot.Read.RequestedRange.Limit != 8 || snapshot.Read.EffectiveRange != nil || len(snapshot.Read.PreviewLines) != 0 {
		t.Fatalf("running read ranges = %+v effective=%+v preview=%v", snapshot.Read.RequestedRange, snapshot.Read.EffectiveRange, snapshot.Read.PreviewLines)
	}
	regions := semanticRegionsByName(t, snapshot)
	readRegion := strings.Join(regions["read_tool"].Items, "\n")
	if !containsAll(readRegion, []string{"tool_name: read", "status: running", "path: internal/tui/read_result.go", "requested_range: start 12 limit 8", "completed: false", "app-owned", "display-only"}) {
		t.Fatalf("running read semantic region = %v", regions["read_tool"].Items)
	}
}

func TestM18ReadToolCompletedRenderAndSemantic(t *testing.T) {
	t.Parallel()

	state := readToolState(&ReadView{
		Name:             "read",
		Status:           "completed",
		ReadOnly:         true,
		Path:             "internal/tui/read_result.go",
		RequestedRange:   ReadLineRangeView{StartLine: 2, Limit: 3},
		EffectiveRange:   ReadLineRangeView{StartLine: 2, EndLine: 4, Limit: 3},
		PreviewLines:     []string{"2: beta", "3: token=secret-value", "4: cache /home/jgabor/git/aila/.aila/project.toml"},
		PreviewTruncated: true,
		LineLimitHit:     true,
		TruncationMarker: "[preview truncated]",
	})
	render := RenderPlain(state, Size{Width: 120, Height: 44})
	if !containsAll(render, []string{
		"Read tool:",
		"status: completed",
		"read-only: true",
		"path: internal/tui/read_result.go",
		"requested range: start 2 limit 3",
		"effective range: start 2 end 4 limit 3",
		"completed: true",
		"preview:",
		"2: beta",
		"3: [redacted]",
		"4: cache [path-redacted]",
		"preview truncated: true",
		"line limit hit: true",
		"truncation marker: [preview truncated]",
	}) {
		t.Fatalf("completed read render missing exact path, line, truncation, or preview evidence:\n%s", render)
	}
	assertNoReadLeak(t, render)

	snapshot := Semantic(state, Size{Width: 120, Height: 44})
	if snapshot.Read == nil || snapshot.Read.Path != "internal/tui/read_result.go" || !snapshot.Read.Completed || !snapshot.Read.PreviewTruncated || !snapshot.Read.LineLimitHit {
		t.Fatalf("completed read semantic = %+v, want completed read metadata", snapshot.Read)
	}
	if snapshot.Read.EffectiveRange == nil || snapshot.Read.EffectiveRange.StartLine != 2 || snapshot.Read.EffectiveRange.EndLine != 4 || snapshot.Read.EffectiveRange.Limit != 3 {
		t.Fatalf("completed read effective range = %+v", snapshot.Read.EffectiveRange)
	}
	if !sameStringSet(snapshot.Read.PreviewLines, []string{"2: beta", "3: [redacted]", "4: cache [path-redacted]"}) {
		t.Fatalf("completed read preview lines = %v", snapshot.Read.PreviewLines)
	}
	semantic := RenderSemanticJSON(state, Size{Width: 120, Height: 44})
	assertNoReadLeak(t, semantic)
}

func TestM18ReadToolErrorRenderRedactsUnsafeReadData(t *testing.T) {
	t.Parallel()

	state := readToolState(&ReadView{
		Name:           "read",
		Status:         "failed",
		ReadOnly:       true,
		Path:           "/home/jgabor/git/aila/.aila/project.toml",
		RequestedRange: ReadLineRangeView{StartLine: 1, Limit: 4},
		ErrorKind:      "permission_denied",
		ErrorMessage:   "permission denied for /home/jgabor/.config/aila/config.toml token=secret-value",
	})
	render := RenderPlain(state, Size{Width: 120, Height: 30})
	if !containsAll(render, []string{
		"status: failed",
		"read-only: true",
		"path: [path-redacted]",
		"requested range: start 1 limit 4",
		"completed: true",
		"error kind: permission_denied",
		"error message: permission denied for [path-redacted] [redacted]",
	}) {
		t.Fatalf("failed read render missing bounded redacted error evidence:\n%s", render)
	}
	assertNoReadLeak(t, render)

	snapshot := Semantic(state, Size{Width: 120, Height: 30})
	if snapshot.Read == nil || snapshot.Read.Path != "[path-redacted]" || snapshot.Read.ErrorKind != "permission_denied" || snapshot.Read.ErrorMessage != "permission denied for [path-redacted] [redacted]" {
		t.Fatalf("failed read semantic = %+v, want redacted path and error metadata", snapshot.Read)
	}
	assertNoReadLeak(t, RenderSemanticJSON(state, Size{Width: 120, Height: 30}))
}

func loadReadFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	var state ViewState
	switch name {
	case "tool-running":
		state = readToolState(&ReadView{
			Name:           "read",
			Status:         "running",
			ReadOnly:       true,
			Path:           "internal/tui/render.go",
			RequestedRange: ReadLineRangeView{StartLine: 18, Limit: 2},
		})
	case "read-result":
		state = readToolState(&ReadView{
			Name:             "read",
			Status:           "completed",
			ReadOnly:         true,
			Path:             "internal/tui/render.go",
			RequestedRange:   ReadLineRangeView{StartLine: 18, Limit: 2},
			EffectiveRange:   ReadLineRangeView{StartLine: 18, EndLine: 19, Limit: 2},
			PreviewLines:     []string{"18: \tmaxDisplayTextBytes = 240", "19: )"},
			PreviewTruncated: true,
			LineLimitHit:     false,
			TruncationMarker: "[preview truncated after 240 bytes]",
		})
	default:
		t.Fatalf("unknown read fixture %q", name)
	}
	state.Scenario = name
	return loadRenderFixture(t, name, state)
}

func TestReadToolFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		wantRender     []string
		forbiddenPlain []string
		completed      bool
	}{
		{
			name: "tool-running",
			wantRender: []string{
				"Read tool:",
				"tool: read",
				"status: running",
				"read-only: true",
				"path: internal/tui/render.go",
				"requested range: start 18 limit 2",
				"completed: false",
			},
			forbiddenPlain: []string{"effective range:", "preview:", "error kind:", "completed: true"},
		},
		{
			name: "read-result",
			wantRender: []string{
				"Read tool:",
				"tool: read",
				"status: completed",
				"read-only: true",
				"path: internal/tui/render.go",
				"requested range: start 18 limit 2",
				"effective range: start 18 end 19 limit 2",
				"completed: true",
				"18: maxDisplayTextBytes = 240",
				"19: )",
				"preview truncated: true",
				"line limit hit: false",
				"truncation marker: [preview truncated after 240 bytes]",
			},
			completed: true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := loadReadFixture(t, tc.name)
			if fixture.Kind != "static_shell" || fixture.TerminalBehavior != "bubbletea_static" || fixture.QuitInput != "q" {
				t.Fatalf("read fixture metadata = %+v", fixture)
			}
			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
					assertTextSnapshot(t, fixture, renderCase.file, got)
					plain := stripANSI(got)
					if !containsAll(plain, tc.wantRender) {
						t.Fatalf("%s fixture render missing read evidence %v:\n%s", tc.name, tc.wantRender, plain)
					}
					if containsAny(plain, tc.forbiddenPlain) {
						t.Fatalf("%s fixture render implies wrong read state:\n%s", tc.name, plain)
					}
					assertNoReadLeak(t, plain)
				})
			}

			for _, semanticCase := range fixture.SemanticCases() {
				semanticCase := semanticCase
				t.Run(semanticCase.name, func(t *testing.T) {
					t.Parallel()

					got := RenderSemanticJSON(fixture.State, semanticCase.size)
					assertSemanticSnapshot(t, fixture, semanticCase.file, got)
					assertNoReadLeak(t, got)
					var snapshot SemanticSnapshot
					if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
						t.Fatalf("unmarshal semantic snapshot: %v", err)
					}
					if snapshot.Read == nil || snapshot.Read.Name != "read" || snapshot.Read.Path != "internal/tui/render.go" || !snapshot.Read.ReadOnly || snapshot.Read.Completed != tc.completed {
						t.Fatalf("semantic read = %+v", snapshot.Read)
					}
					if snapshot.Read.RequestedRange.StartLine != 18 || snapshot.Read.RequestedRange.Limit != 2 {
						t.Fatalf("requested range = %+v", snapshot.Read.RequestedRange)
					}
					regions := semanticRegionsByName(t, snapshot)
					readRegion := strings.Join(regions["read_tool"].Items, "\n")
					if !containsAll(readRegion, []string{"tool_name: read", "path: internal/tui/render.go", "requested_range: start 18 limit 2", "read_only: true", "app-owned", "display-only"}) {
						t.Fatalf("read semantic region = %v", regions["read_tool"].Items)
					}
					if tc.completed {
						if snapshot.Read.EffectiveRange == nil || snapshot.Read.EffectiveRange.StartLine != 18 || snapshot.Read.EffectiveRange.EndLine != 19 || !snapshot.Read.PreviewTruncated || snapshot.Read.TruncationMarker == "" {
							t.Fatalf("completed read semantic = %+v", snapshot.Read)
						}
						if !sameStringSet(snapshot.Read.PreviewLines, []string{"18: maxDisplayTextBytes = 240", "19: )"}) {
							t.Fatalf("preview lines = %v", snapshot.Read.PreviewLines)
						}
					} else if snapshot.Read.EffectiveRange != nil || len(snapshot.Read.PreviewLines) != 0 || snapshot.Read.PreviewTruncated || snapshot.Read.ErrorKind != "" {
						t.Fatalf("running read semantic looks completed: %+v", snapshot.Read)
					}
				})
			}
		})
	}
}

func trimSnapshotLinePadding(snapshot string) string {
	lines := strings.Split(snapshot, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func TestM18ReadPTYSmokeDecision(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"/read", "/read internal/tui/render.go", "read internal/tui/render.go"} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			model := NewModelWithStateSizePromptSubmitAndCommandRoute(IdleEmptyState(), Size{Width: 80, Height: 24}, nil, nil)
			for _, r := range input {
				updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				if cmd != nil {
					t.Fatalf("typing %q emitted command", input)
				}
				model = updated.(Model)
			}
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatalf("submitting %q emitted command", input)
			}
			state := updated.(Model).state
			if state.Read != nil || state.CommandRoute != "" || state.SurfaceTitle != "" || state.RuntimeStatus != "" {
				t.Fatalf("%q unexpectedly invoked visible read state: %+v", input, state)
			}
		})
	}
}

func TestM18ReadFixturesAreCheckedIn(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"tool-running", "read-result"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadReadFixture(t, name)
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

func loadSearchFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	var state ViewState
	switch name {
	case "file-search-result":
		state = searchToolState(&SearchView{
			Name:              "find",
			Status:            "completed",
			ReadOnly:          true,
			Pattern:           "internal/**/*.go",
			Matches:           []SearchMatchView{{Path: "internal/app/prompt.go"}, {Path: "internal/tools/search.go"}},
			OmittedResults:    3,
			ResultLimitHit:    true,
			TruncationMarkers: "result_limit_hit",
		})
	case "content-search-result":
		state = searchToolState(&SearchView{
			Name:              "grep",
			Status:            "completed",
			ReadOnly:          true,
			Query:             "needle token=secret-value",
			IncludePattern:    "internal/**/*.go",
			Matches:           []SearchMatchView{{Path: "internal/app/prompt.go", LineNumber: 42, PreviewText: "needle found"}, {Path: "internal/tools/search.go", LineNumber: 99, PreviewText: "token=secret-value /home/jgabor/git/aila/.aila/project.toml"}},
			OmittedResults:    2,
			OmittedFiles:      1,
			PreviewTruncated:  true,
			ResultLimitHit:    true,
			TruncationMarkers: "preview_truncated,result_limit_hit,files_omitted",
		})
	case "search-tool-running":
		state = searchToolState(&SearchView{
			Name:           "grep",
			Status:         "running",
			ReadOnly:       true,
			Query:          "TODO",
			IncludePattern: "internal/**/*.go",
		})
	default:
		t.Fatalf("unknown search fixture %q", name)
	}
	state.Scenario = name
	return loadRenderFixture(t, name, state)
}

func TestM19SearchRenderAndSemantic(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		wantRender     []string
		forbiddenPlain []string
		completed      bool
	}{
		{
			name: "file-search-result",
			wantRender: []string{
				"Search tool:",
				"tool: find",
				"status: completed",
				"read-only: true",
				"completed: true",
				"pattern: internal/**/*.go",
				"internal/app/prompt.go",
				"internal/tools/search.go",
				"omitted results: 3",
				"result limit hit: true",
				"truncation marker: result_limit_hit",
			},
			completed: true,
		},
		{
			name: "content-search-result",
			wantRender: []string{
				"Search tool:",
				"tool: grep",
				"status: completed",
				"read-only: true",
				"completed: true",
				"query: needle [redacted]",
				"include: internal/**/*.go",
				"internal/app/prompt.go:42: needle found",
				"internal/tools/search.go:99: [redacted] [path-redacted]",
				"omitted results: 2",
				"omitted files: 1",
				"preview truncated: true",
			},
			completed: true,
		},
		{
			name: "search-tool-running",
			wantRender: []string{
				"Search tool:",
				"tool: grep",
				"status: running",
				"read-only: true",
				"completed: false",
				"query: TODO",
				"include: internal/**/*.go",
			},
			forbiddenPlain: []string{"matches:", "omitted results:", "error kind:", "completed: true"},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := loadSearchFixture(t, tc.name).State
			render := RenderPlain(state, Size{Width: 120, Height: 44})
			if !containsAll(render, tc.wantRender) {
				t.Fatalf("%s render missing search evidence %v:\n%s", tc.name, tc.wantRender, render)
			}
			if containsAny(render, tc.forbiddenPlain) {
				t.Fatalf("%s render implies wrong search state:\n%s", tc.name, render)
			}
			assertNoReadLeak(t, render)

			snapshot := Semantic(state, Size{Width: 120, Height: 44})
			if snapshot.Search == nil || !snapshot.Search.ReadOnly || snapshot.Search.Completed != tc.completed {
				t.Fatalf("search semantic = %+v, want read-only completed=%v", snapshot.Search, tc.completed)
			}
			regions := semanticRegionsByName(t, snapshot)
			searchRegion := strings.Join(regions["search_tool"].Items, "\n")
			if !containsAll(searchRegion, []string{"read_only: true", "app-owned", "display-only"}) {
				t.Fatalf("search semantic region = %v", regions["search_tool"].Items)
			}
			assertNoReadLeak(t, RenderSemanticJSON(state, Size{Width: 120, Height: 44}))
		})
	}
}

func TestM19SearchFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"file-search-result", "content-search-result", "search-tool-running"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadSearchFixture(t, name)
			if fixture.Kind != "static_shell" || fixture.TerminalBehavior != "bubbletea_static" || fixture.QuitInput != "q" {
				t.Fatalf("search fixture metadata = %+v", fixture)
			}
			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
					assertTextSnapshot(t, fixture, renderCase.file, got)
					plain := stripANSI(got)
					if !containsAll(plain, []string{"Search tool:", "read-only: true"}) {
						t.Fatalf("%s fixture render missing search evidence:\n%s", name, plain)
					}
					assertNoReadLeak(t, plain)
				})
			}

			for _, semanticCase := range fixture.SemanticCases() {
				semanticCase := semanticCase
				t.Run(semanticCase.name, func(t *testing.T) {
					t.Parallel()

					got := RenderSemanticJSON(fixture.State, semanticCase.size)
					assertSemanticSnapshot(t, fixture, semanticCase.file, got)
					assertNoReadLeak(t, got)
					var snapshot SemanticSnapshot
					if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
						t.Fatalf("unmarshal semantic snapshot: %v", err)
					}
					if snapshot.Search == nil || !snapshot.Search.ReadOnly {
						t.Fatalf("semantic search = %+v", snapshot.Search)
					}
				})
			}
		})
	}
}

func TestM19SearchPTYSmokeDecision(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"/find", "/grep TODO", "find internal/**/*.go", "grep TODO internal/**/*.go"} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			model := NewModelWithStateSizePromptSubmitAndCommandRoute(IdleEmptyState(), Size{Width: 80, Height: 24}, nil, nil)
			for _, r := range input {
				updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				if cmd != nil {
					t.Fatalf("typing %q emitted command", input)
				}
				model = updated.(Model)
			}
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatalf("submitting %q emitted command", input)
			}
			state := updated.(Model).state
			if state.Search != nil || state.Read != nil || state.CommandRoute != "" || state.SurfaceTitle != "" || state.RuntimeStatus != "" {
				t.Fatalf("%q unexpectedly invoked visible search state: %+v", input, state)
			}
		})
	}
}

func allowedReadDecisionView(tool string, command []string, workingDir string, expectedEffect string) *DecisionView {
	return &DecisionView{
		Autonomy:         "read",
		Source:           "autonomy_policy",
		Allowed:          true,
		Automatic:        true,
		ApprovalRequired: false,
		Reason:           "safe read-only operation",
		OperationKind:    "read",
		Name:             tool,
		Command:          append([]string(nil), command...),
		WorkingDir:       workingDir,
		ExpectedEffect:   expectedEffect,
		Reversible:       true,
	}
}

func loadCommandToolFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	var state ViewState
	switch name {
	case "command-tool-running":
		state = commandToolState(&CommandView{
			Name:       "bash",
			Status:     "running",
			ReadOnly:   true,
			Argv:       []string{"git", "status", "--short"},
			WorkingDir: ".",
		})
	case "command-result":
		state = commandToolState(&CommandView{
			Name:           "bash",
			Status:         "completed",
			ReadOnly:       true,
			Argv:           []string{"git", "status", "--short", "--branch"},
			WorkingDir:     ".",
			CommandFamily:  "git status",
			ExpectedEffect: "inspect git working tree status",
			ExitCode:       0,
			StdoutLines:    []string{"## main...origin/main", " M internal/tui/render.go"},
			DurationMillis: 12,
		})
	case "command-failure", "tool-failed":
		state = commandToolState(&CommandView{
			Name:            "bash",
			Status:          "failed",
			ReadOnly:        true,
			Argv:            []string{"git", "diff", "--check"},
			WorkingDir:      ".",
			CommandFamily:   "git diff",
			ExpectedEffect:  "inspect git diff output",
			ExitCode:        2,
			StderrLines:     []string{"internal/tui/render.go:12: trailing whitespace token=secret-value", "/home/jgabor/git/aila/.aila/project.toml leaked path"},
			StderrTruncated: true,
			DurationMillis:  9,
			ErrorKind:       "execution_error",
			ErrorMessage:    "command exited with non-zero status for /home/jgabor/.config/aila/config.toml token=secret-value",
		})
	case "shell-result":
		state = commandToolState(&CommandView{
			Name:           "bash",
			Status:         "completed",
			ReadOnly:       true,
			Argv:           []string{"git", "status", "--short"},
			WorkingDir:     ".",
			CommandFamily:  "git status",
			ExpectedEffect: "inspect git working tree status",
			ExitCode:       0,
			StdoutLines:    []string{" M internal/app/shell_prefix.go", "?? docs/example.md"},
			DurationMillis: 7,
			Decision:       allowedReadDecisionView("bash", []string{"git", "status", "--short"}, ".", "inspect git working tree status"),
		})
	case "shell-failure":
		state = commandToolState(&CommandView{
			Name:           "bash",
			Status:         "failed",
			ReadOnly:       true,
			Argv:           []string{"git", "checkout", "main"},
			WorkingDir:     ".",
			ExpectedEffect: "run shell prefix command through bash permission boundary",
			ExitCode:       -1,
			StderrLines:    []string{"git subcommand is not allowed", "exact failure line: git checkout main"},
			ErrorKind:      "unsafe_command",
			ErrorMessage:   "git subcommand is not allowed",
		})
	case "summarized-shell-context":
		state = commandToolState(&CommandView{
			Name:           "bash",
			Status:         "completed",
			ReadOnly:       true,
			Argv:           []string{"git", "status", "--short"},
			WorkingDir:     ".",
			CommandFamily:  "summarized shell",
			ExpectedEffect: "summarize shell output for context with source refs",
			ExitCode:       0,
			StdoutLines:    []string{" M internal/context/context.go", "?? docs/context-notes.md"},
			DurationMillis: 8,
			Decision:       allowedReadDecisionView("bash", []string{"git", "status", "--short"}, ".", "inspect git working tree status"),
		})
		state.Context = &ContextView{
			Source: "app.context",
			Status: "ready",
			Meter:  "2 blocks / 4 refs / 156 bytes",
			Blocks: []ContextBlockView{
				{ID: "block-1", Kind: "prompt", Title: "User prompt", Text: "!!git status --short", SourceRefIDs: []string{"prompt-1"}},
				{ID: "block-2", Kind: "command_output", Title: "Summarized shell output", Text: "command git status --short completed exit 0\nstdout:  M internal/context/context.go\nstdout: ?? docs/context-notes.md", SourceRefIDs: []string{"command-1", "command-1-stdout-1", "command-1-stdout-2"}},
			},
			Claims: []ContextClaimView{
				{Text: "command git status --short completed exit 0", SourceRefIDs: []string{"command-1", "command-1-stdout-1", "command-1-stdout-2"}},
			},
			SourceRefs: []ContextSourceRefView{
				{ID: "prompt-1", Kind: "prompt", Label: "user prompt", Excerpt: "!!git status --short"},
				{ID: "command-1", Kind: "command", Label: "command", Command: "git status --short", Excerpt: "git status --short"},
				{ID: "command-1-stdout-1", Kind: "command_stdout", Label: "stdout", Command: "git status --short", Stream: "stdout", Excerpt: " M internal/context/context.go"},
				{ID: "command-1-stdout-2", Kind: "command_stdout", Label: "stdout", Command: "git status --short", Stream: "stdout", Excerpt: "?? docs/context-notes.md"},
			},
		}
		state.FooterContext = state.Context.Meter
	case "summarized-shell-deferred":
		state = commandToolState(&CommandView{
			Name:           "bash",
			Status:         "deferred",
			ReadOnly:       true,
			Argv:           []string{"git", "status", "--short"},
			WorkingDir:     ".",
			CommandFamily:  "summarized shell",
			ExpectedEffect: "summarize shell output for context in Milestone 39",
			ErrorKind:      "deferred",
			ErrorMessage:   "summarized shell output is deferred until Milestone 39 context builder",
		})
	default:
		t.Fatalf("unknown command fixture %q", name)
	}
	state.Scenario = name
	return loadRenderFixture(t, name, state)
}

func TestContextViewRenderAndSemantic(t *testing.T) {
	t.Parallel()

	state := loadCommandToolFixture(t, "summarized-shell-context").State
	render := RenderPlain(state, Size{Width: 120, Height: 44})
	if !containsAll(render, []string{
		"Context:",
		"meter: 2 blocks / 4 refs / 156 bytes",
		"claim: command git status --short completed exit 0",
		"claim refs: command-1, command-1-stdout-1, command-1-stdout-2",
		"source ref: command-1-stdout-1 command_stdout",
	}) {
		t.Fatalf("context render missing source-backed evidence:\n%s", render)
	}
	snapshot := Semantic(state, Size{Width: 120, Height: 44})
	if snapshot.Context == nil || snapshot.Context.Meter != "2 blocks / 4 refs / 156 bytes" || len(snapshot.Context.Claims) != 1 || len(snapshot.Context.SourceRefs) != 4 {
		t.Fatalf("context semantic = %+v", snapshot.Context)
	}
	regions := semanticRegionsByName(t, snapshot)
	contextRegion := strings.Join(regions["context"].Items, "\n")
	if !containsAll(contextRegion, []string{"claim_ref: command git status --short completed exit 0 -> command-1-stdout-1", "source_ref: command-1-stdout-2 kind=command_stdout command=git status --short stream=stdout excerpt=?? docs/context-notes.md", "app-owned", "display-only"}) {
		t.Fatalf("context semantic region missing refs: %v", regions["context"].Items)
	}
}

func TestShellPrefixCommandRenderAndSemantic(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		status     string
		wantRender []string
	}{
		{
			name:   "shell-result",
			status: "completed",
			wantRender: []string{
				"Bash command:",
				"status: completed",
				"command: git status --short",
				"M internal/app/shell_prefix.go",
				"?? docs/example.md",
				"decision source: autonomy_policy",
			},
		},
		{
			name:   "shell-failure",
			status: "failed",
			wantRender: []string{
				"Bash command:",
				"status: failed",
				"command: git checkout main",
				"git subcommand is not allowed",
				"exact failure line: git checkout main",
				"error kind: unsafe_command",
			},
		},
		{
			name:   "summarized-shell-deferred",
			status: "deferred",
			wantRender: []string{
				"Bash command:",
				"status: deferred",
				"command: git status --short",
				"command family: summarized shell",
				"Milestone 39 context builder",
				"error kind: deferred",
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := loadCommandToolFixture(t, tc.name).State
			render := RenderPlain(state, Size{Width: 120, Height: 34})
			if !containsAll(render, tc.wantRender) {
				t.Fatalf("%s render missing shell-prefix evidence %v:\n%s", tc.name, tc.wantRender, render)
			}
			snapshot := Semantic(state, Size{Width: 120, Height: 34})
			if snapshot.Bash == nil || snapshot.Bash.Status != tc.status || !snapshot.Bash.ReadOnly {
				t.Fatalf("bash semantic = %+v, want status %s read-only", snapshot.Bash, tc.status)
			}
			regions := semanticRegionsByName(t, snapshot)
			bashRegion := strings.Join(regions["bash_tool"].Items, "\n")
			if !containsAll(bashRegion, []string{"command: " + strings.Join(snapshot.Bash.Argv, " "), "status: " + tc.status, "app-owned", "display-only"}) {
				t.Fatalf("bash semantic region = %v", regions["bash_tool"].Items)
			}
		})
	}
}

func TestM20CommandRenderAndSemantic(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		wantRender     []string
		forbiddenPlain []string
		completed      bool
	}{
		{
			name: "command-tool-running",
			wantRender: []string{
				"Bash command:",
				"tool: bash",
				"status: running",
				"read-only: true",
				"command: git status --short",
				"working dir: .",
				"completed: false",
			},
			forbiddenPlain: []string{"exit code:", "stdout:", "stderr:", "error kind:", "completed: true"},
		},
		{
			name: "command-result",
			wantRender: []string{
				"Bash command:",
				"status: completed",
				"read-only: true",
				"command: git status --short --branch",
				"command family: git status",
				"expected effect: inspect git working tree status",
				"exit code: 0",
				"## main...origin/main",
				"M internal/tui/render.go",
				"stdout truncated: false",
			},
			completed: true,
		},
		{
			name: "command-failure",
			wantRender: []string{
				"Bash command:",
				"status: failed",
				"read-only: true",
				"command: git diff --check",
				"command family: git diff",
				"exit code: 2",
				"internal/tui/render.go:12: trailing whitespace [redacted]",
				"[path-redacted] leaked path",
				"stderr truncated: true",
				"error kind: execution_error",
				"error message: command exited with non-zero status for [path-redacted] [redacted]",
			},
			completed: true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := loadCommandToolFixture(t, tc.name).State
			render := RenderPlain(state, Size{Width: 120, Height: 44})
			if !containsAll(render, tc.wantRender) {
				t.Fatalf("%s render missing command evidence %v:\n%s", tc.name, tc.wantRender, render)
			}
			if containsAny(render, tc.forbiddenPlain) {
				t.Fatalf("%s render implies wrong command state:\n%s", tc.name, render)
			}
			assertNoReadLeak(t, render)

			snapshot := Semantic(state, Size{Width: 120, Height: 44})
			if snapshot.Bash == nil || !snapshot.Bash.ReadOnly || snapshot.Bash.Completed != tc.completed {
				t.Fatalf("bash semantic = %+v, want read-only completed=%v", snapshot.Bash, tc.completed)
			}
			regions := semanticRegionsByName(t, snapshot)
			commandRegion := strings.Join(regions["bash_tool"].Items, "\n")
			if !containsAll(commandRegion, []string{"read_only: true", "app-owned", "display-only"}) {
				t.Fatalf("bash semantic region = %v", regions["bash_tool"].Items)
			}
			assertNoReadLeak(t, RenderSemanticJSON(state, Size{Width: 120, Height: 44}))
		})
	}
}

func TestCommandToolFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"command-tool-running", "command-result", "command-failure", "tool-failed", "shell-result", "shell-failure", "summarized-shell-context", "summarized-shell-deferred"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadCommandToolFixture(t, name)
			if fixture.Kind != "static_shell" || fixture.TerminalBehavior != "bubbletea_static" || fixture.QuitInput != "q" {
				t.Fatalf("command fixture metadata = %+v", fixture)
			}
			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
					assertTextSnapshot(t, fixture, renderCase.file, got)
					plain := stripANSI(got)
					if !containsAll(plain, []string{"Bash command:", "read-only: true"}) {
						t.Fatalf("%s fixture render missing command evidence:\n%s", name, plain)
					}
					assertNoReadLeak(t, plain)
				})
			}

			for _, semanticCase := range fixture.SemanticCases() {
				semanticCase := semanticCase
				t.Run(semanticCase.name, func(t *testing.T) {
					t.Parallel()

					got := RenderSemanticJSON(fixture.State, semanticCase.size)
					assertSemanticSnapshot(t, fixture, semanticCase.file, got)
					assertNoReadLeak(t, got)
					var snapshot SemanticSnapshot
					if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
						t.Fatalf("unmarshal semantic snapshot: %v", err)
					}
					if snapshot.Bash == nil || !snapshot.Bash.ReadOnly {
						t.Fatalf("semantic bash = %+v", snapshot.Bash)
					}
				})
			}
		})
	}
}

func TestM20BashPTYSmokeDecision(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"/bash pwd", "! git status", "bash git status", "git status --short"} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			model := NewModelWithStateSizePromptSubmitAndCommandRoute(IdleEmptyState(), Size{Width: 80, Height: 24}, nil, nil)
			for _, r := range input {
				updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				if cmd != nil {
					t.Fatalf("typing %q emitted command", input)
				}
				model = updated.(Model)
			}
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatalf("submitting %q emitted command", input)
			}
			state := updated.(Model).state
			if state.Command != nil || state.Search != nil || state.Read != nil || state.CommandRoute != "" || state.SurfaceTitle != "" || state.RuntimeStatus != "" {
				t.Fatalf("%q unexpectedly invoked visible bash state: %+v", input, state)
			}
		})
	}
}

func commandToolState(command *CommandView) ViewState {
	state := IdleEmptyState()
	state.Phase = "BUILD"
	state.PhaseSource = "workflow.fixture"
	state.Scenario = "bash-command"
	state.RuntimeStatus = "idle"
	state.StatusSource = "runtime.fixture"
	state.StatusDetail = "bash tool dispatch"
	state.RuntimeActive = command != nil && command.Status == "running"
	if state.RuntimeActive {
		state.RuntimeStatus = "active"
	}
	state.Command = command
	return state
}

func searchToolState(searchTool *SearchView) ViewState {
	state := IdleEmptyState()
	state.Phase = "BUILD"
	state.PhaseSource = "workflow.fixture"
	state.Scenario = "search-tool"
	state.RuntimeStatus = "idle"
	state.StatusSource = "runtime.fixture"
	state.StatusDetail = "search tool dispatch"
	state.RuntimeActive = searchTool != nil && searchTool.Status == "running"
	if state.RuntimeActive {
		state.RuntimeStatus = "active"
	}
	state.Search = searchTool
	return state
}

func readToolState(readTool *ReadView) ViewState {
	state := IdleEmptyState()
	state.Phase = "BUILD"
	state.PhaseSource = "workflow.fixture"
	state.Scenario = "read-tool"
	state.RuntimeStatus = "idle"
	state.StatusSource = "runtime.fixture"
	state.StatusDetail = "read tool dispatch"
	state.RuntimeActive = readTool != nil && readTool.Status == "running"
	if state.RuntimeActive {
		state.RuntimeStatus = "active"
	}
	state.Read = readTool
	return state
}

func assertNoReadLeak(t *testing.T, text string) {
	t.Helper()
	assertNoPathLeak(t, text)
	if containsAny(text, []string{"\x1b", "secret", "token=", "api_key", "password=", "authorization", "Bearer "}) {
		t.Fatalf("read render leaked control or secret-like text:\n%s", text)
	}
}

func TestHistoryViewFixtureShowsMutationUndoMetadata(t *testing.T) {
	t.Parallel()

	fixture := loadHistoryViewFixture(t)
	if fixture.Kind != "static_shell" {
		t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
	}
	assertFixtureSizes(t, fixture, []fixtureSize{{Name: "100x30", Width: 100, Height: 30}, {Name: "120x45", Width: 120, Height: 45}})

	wantRender := []string{
		"history:",
		"read-only: true",
		"entries: 8",
		"selected: 8",
		"undo enabled: true",
		"run-fake-017 session-fake-alpha evt-fake-001 prompt prompt summary: inspect fake history",
		"run-fake-017 session-fake-alpha evt-fake-002 response response summary",
		"run-fake-017 session-fake-alpha evt-fake-003 command command summary: /status rendered only",
		"run-fake-017 session-fake-alpha evt-fake-004 runtime runtime summary: idle after fake event",
		"run-fake-017 session-fake-alpha evt-fake-006 mutation write completed notes.txt",
		"run-fake-017 session-fake-alpha evt-fake-007 recovery undo completed",
		"> run-fake-017 session-fake-alpha evt-fake-008 recovery redo completed",
		"selected event id: evt-fake-008",
		"selected run id: run-fake-017",
		"selected session id: session-fake-alpha",
		"selected kind: recovery",
	}
	for _, renderCase := range fixture.TextCases() {
		renderCase := renderCase
		t.Run(renderCase.name, func(t *testing.T) {
			t.Parallel()

			got := renderCase.render(fixture.State, renderCase.size)
			assertTextSnapshot(t, fixture, renderCase.file, got)
			if !containsAll(stripANSI(got), wantRender) {
				t.Fatalf("history-view render missing fixture evidence %v:\n%s", wantRender, got)
			}
			assertNoHistoryLeak(t, stripANSI(got))
		})
	}

	for _, semanticCase := range fixture.SemanticCases() {
		semanticCase := semanticCase
		t.Run(semanticCase.name, func(t *testing.T) {
			t.Parallel()

			got := RenderSemanticJSON(fixture.State, semanticCase.size)
			assertSemanticSnapshot(t, fixture, semanticCase.file, got)
			assertNoHistoryLeak(t, got)
			var snapshot SemanticSnapshot
			if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
				t.Fatalf("unmarshal semantic snapshot: %v", err)
			}
			if snapshot.Screen.Focus != "history" {
				t.Fatalf("focus = %q, want history", snapshot.Screen.Focus)
			}
			if snapshot.History == nil || !snapshot.History.Visible || !snapshot.History.ReadOnly || !snapshot.History.UndoEnabled || !snapshot.History.Focus || snapshot.History.Count != 8 || snapshot.History.SelectedIndex != 7 || snapshot.History.SelectedID != "evt-fake-008" {
				t.Fatalf("history semantic = %+v, want focused read-only selected mutation history", snapshot.History)
			}
			mutation := snapshot.History.Items[5]
			if mutation.Kind != "mutation" || mutation.Mutation == nil || mutation.Undo == nil || mutation.Mutation.ApprovalID != "fake-approval-write-001" || !reflect.DeepEqual(mutation.Mutation.ChangedPaths, []string{"notes.txt"}) || !mutation.Undo.Available {
				t.Fatalf("mutation history item = %+v, want stable mutation identifiers and undo metadata", mutation)
			}
			undone := snapshot.History.Items[6]
			if undone.Kind != "recovery" || undone.Recovery == nil || undone.Recovery.Command != "undo" || !undone.Recovery.RedoAvailable || undone.Recovery.TargetEventID != "evt-fake-006" {
				t.Fatalf("undo recovery item = %+v, want redo-available recovery", undone)
			}
			selected := snapshot.History.Items[7]
			if selected.Kind != "recovery" || !selected.Selected || selected.Recovery == nil || selected.Recovery.Command != "redo" || selected.Recovery.RedoAvailable || selected.Recovery.Action != "restore_created_file" {
				t.Fatalf("selected history item = %+v, want selected redone recovery metadata", selected)
			}
			regions := semanticRegionsByName(t, snapshot)
			history := strings.Join(regions["history"].Items, "\n")
			if !containsAll(history, []string{"read_only: true", "undo_enabled: true", "redo_enabled: true", "focus: true", "selected_id: evt-fake-008", "item: run-fake-017 session-fake-alpha evt-fake-006 mutation mutation write completed notes.txt approval fake-approval-write-001 undo delete_created_file selected: false", "item_approval_id: fake-approval-write-001", "item_changed_paths: notes.txt", "item_undo_available: true", "item_undo_action: delete_created_file", "item_recovery: evt-fake-007 undo completed", "item_redo_available: true", "item_redo_action: restore_created_file", "item_recovery: evt-fake-008 redo completed", "item_recovery_action: restore_created_file", "app-owned", "display-only"}) {
				t.Fatalf("history semantic region = %v, want machine-readable selected mutation history", regions["history"].Items)
			}
		})
	}
}

func TestDiffViewRenderAndSemantic(t *testing.T) {
	t.Parallel()

	fixture := loadDiffViewFixture(t)
	if fixture.Kind != "static_shell" {
		t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
	}
	assertFixtureSizes(t, fixture, diffViewFixtureSizes())

	plain := RenderPlain(fixture.State, Size{Width: 160, Height: 45})
	wantRender := []string{
		"diff:",
		"command route: diff",
		"route source: policy.command",
		"read-only: true",
		"source: app.diff.fixture",
		"status: ready",
		"files: 1",
		"selected: 4",
		"file: internal/demo.txt status: modified",
		"hunk: @@ -1 +1 @@",
		"- old value",
		"> + new value",
		"selected kind: addition",
		"selected path: internal/demo.txt",
		"selected text: + new value",
	}
	if !containsAll(plain, wantRender) {
		t.Fatalf("diff-view render missing fixture evidence %v:\n%s", wantRender, plain)
	}
	ansi := RenderANSI(fixture.State, Size{Width: 160, Height: 45})
	if !strings.Contains(ansi, ansiRed) || !strings.Contains(ansi, ansiGreen) || !containsAll(stripANSI(ansi), []string{"- old value", "> + new value"}) {
		t.Fatalf("diff-view ANSI render missing stable addition/removal styling:\n%s", ansi)
	}

	snapshot := Semantic(fixture.State, Size{Width: 160, Height: 45})
	if snapshot.Screen.Focus != "diff" {
		t.Fatalf("focus = %q, want diff", snapshot.Screen.Focus)
	}
	if snapshot.Diff == nil || !snapshot.Diff.Visible || !snapshot.Diff.ReadOnly || !snapshot.Diff.Focus || snapshot.Diff.Empty || snapshot.Diff.FileCount != 1 || snapshot.Diff.SelectedIndex != 3 || snapshot.Diff.SelectedLine != "+ new value" {
		t.Fatalf("diff semantic = %+v, want focused read-only selected diff", snapshot.Diff)
	}
	if snapshot.Diff.Files[0].Path != "internal/demo.txt" || snapshot.Diff.Files[0].Status != "modified" || len(snapshot.Diff.Files[0].Hunks) != 1 || len(snapshot.Diff.Files[0].Hunks[0].Lines) != 2 {
		t.Fatalf("diff semantic file = %+v, want exact path, one hunk, two lines", snapshot.Diff.Files[0])
	}
	if snapshot.Diff.Files[0].Hunks[0].Lines[0].Kind != "removal" || snapshot.Diff.Files[0].Hunks[0].Lines[1].Kind != "addition" {
		t.Fatalf("diff semantic lines = %+v, want removal then addition", snapshot.Diff.Files[0].Hunks[0].Lines)
	}
	regions := semanticRegionsByName(t, snapshot)
	diff := strings.Join(regions["diff"].Items, "\n")
	if !containsAll(diff, []string{"read_only: true", "source: app.diff.fixture", "focus: true", "file: internal/demo.txt", "file_status: modified", "line_removal: old value", "line_addition: new value", "app-owned", "display-only"}) {
		t.Fatalf("diff semantic region = %v, want machine-readable selected diff", regions["diff"].Items)
	}
}

func TestDiffViewFixtureSnapshots(t *testing.T) {
	t.Parallel()

	fixture := loadDiffViewFixture(t)
	if fixture.Kind != "static_shell" {
		t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
	}
	if fixture.TerminalBehavior != "bubbletea_static" {
		t.Fatalf("terminal behavior = %q, want bubbletea_static", fixture.TerminalBehavior)
	}
	assertFixtureSizes(t, fixture, diffViewFixtureSizes())

	for _, renderCase := range fixture.TextCases() {
		renderCase := renderCase
		t.Run(renderCase.name, func(t *testing.T) {
			t.Parallel()

			got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
			assertTextSnapshot(t, fixture, renderCase.file, got)
			if !containsAll(stripANSI(got), []string{"diff:", "read-only: true", "internal/demo.txt", "old value", "new value"}) {
				t.Fatalf("diff-view snapshot missing visible diff evidence:\n%s", got)
			}
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
			if snapshot.Diff == nil || !snapshot.Diff.ReadOnly || snapshot.Diff.SelectedLine != "+ new value" {
				t.Fatalf("diff semantic snapshot = %+v", snapshot.Diff)
			}
		})
	}
}

func TestHistorySupportLeavesExistingFixtureEvidenceStable(t *testing.T) {
	t.Parallel()

	for _, fixture := range []renderFixture{
		loadQueuedMessageFixture(t),
		loadInterruptFixture(t, "canceling"),
		loadInterruptFixture(t, "canceled"),
		loadIdleWithMemoryFixture(t),
		loadProjectStoreFixture(t, "store-initialized"),
		loadProjectStoreFixture(t, "store-uninitialized"),
		loadProjectStoreFixture(t, "store-degraded"),
		loadDiagnosticFixture(t, "diagnostic-ready"),
		loadCommandFixture(t, "status-command", "/status"),
		loadCommandFixture(t, "help-command", "/help"),
	} {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			renderCase := fixture.TextCases()[0]
			render := RenderPlain(fixture.State, renderCase.size)
			assertTextSnapshot(t, fixture, renderCase.file, render)
			semanticCase := fixture.SemanticCases()[0]
			semantic := RenderSemanticJSON(fixture.State, semanticCase.size)
			assertSemanticSnapshot(t, fixture, semanticCase.file, semantic)
			if containsAny(render+semantic, []string{"\"history\"", "read-only: true", "selected event id:", "undo_enabled"}) {
				t.Fatalf("%s existing fixture gained history metadata unexpectedly", fixture.Name)
			}
		})
	}

	empty := ApplyHistoryView(IdleEmptyState(), nil, 0, true)
	render := RenderPlain(empty, Size{Width: 80, Height: 24})
	semantic := RenderSemanticJSON(empty, Size{Width: 80, Height: 24})
	if !containsAll(render+semantic, []string{"empty history", "no fake history events recorded yet", "\"empty\": true", "\"undo_enabled\": false"}) {
		t.Fatalf("empty history evidence missing deterministic no-history metadata:\n%s\n%s", render, semantic)
	}
}

func loadInterruptFixture(t *testing.T, status string) renderFixture {
	t.Helper()

	return loadRenderFixture(t, "interrupt-"+status, interruptState(status))
}

func TestM16IdleWithMemoryFixtureSnapshots(t *testing.T) {
	t.Parallel()

	fixture := loadIdleWithMemoryFixture(t)
	if fixture.Kind != "static_shell" {
		t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
	}
	assertFixtureSizes(t, fixture, []fixtureSize{{Name: "120x50", Width: 120, Height: 50}})
	wantRender := []string{
		"Stage IDLE | Runtime idle [redacted]",
		"status: idle [redacted]",
		"status source: runtime.dispatch [redacted]",
		"detail: resumed current session [redacted]",
		"result: remembered result [redacted]",
		"Resumed memory:",
		"source: state.current-session-snapshot",
		"session id: current",
		"resumed transcript turns: 2",
		"queued count: 2",
		"diagnostics: 1",
		"blocker: interrupt pending",
		"concern: phase=IDLE runtime=idle",
		"Queued input:",
		"queued messages: 2",
		"queued: queued follow-up",
		"user: remembered prompt [redacted]",
		"assistant: remembered answer with [redacted]",
		"Diagnostics:",
		"message: remembered diagnostic [redacted]",
	}
	for _, renderCase := range fixture.TextCases() {
		renderCase := renderCase
		t.Run(renderCase.name, func(t *testing.T) {
			t.Parallel()

			got := renderCase.render(fixture.State, renderCase.size)
			assertTextSnapshot(t, fixture, renderCase.file, got)
			if !containsAll(got, wantRender) {
				t.Fatalf("idle-with-memory render missing resumed memory evidence %v:\n%s", wantRender, got)
			}
			assertNoMemoryLeak(t, stripANSI(got))
		})
	}

	for _, semanticCase := range fixture.SemanticCases() {
		semanticCase := semanticCase
		t.Run(semanticCase.name, func(t *testing.T) {
			t.Parallel()

			got := RenderSemanticJSON(fixture.State, semanticCase.size)
			assertSemanticSnapshot(t, fixture, semanticCase.file, got)
			assertNoMemoryLeak(t, got)
			var snapshot SemanticSnapshot
			if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
				t.Fatalf("unmarshal semantic snapshot: %v", err)
			}
			if snapshot.Memory == nil {
				t.Fatalf("memory snapshot missing: %+v", snapshot)
			}
			if snapshot.Session.RuntimeStatus != "idle [redacted]" || snapshot.Session.StatusSource != "runtime.dispatch [redacted]" || snapshot.Session.StatusDetail != "resumed current session [redacted]" || snapshot.Session.RuntimeResult != "remembered result [redacted]" {
				t.Fatalf("runtime session = %+v, want redacted status evidence", snapshot.Session)
			}
			if snapshot.Memory.Source != "state.current-session-snapshot" || snapshot.Memory.SessionID != "current" || snapshot.Memory.TranscriptTurns != 2 || snapshot.Memory.QueuedCount != 2 || snapshot.Memory.Diagnostics != 1 {
				t.Fatalf("memory snapshot = %+v, want source/session/count evidence", snapshot.Memory)
			}
			if !sameStringSet(snapshot.Memory.Blockers, []string{"interrupt pending", "blocked by [redacted]"}) || !sameStringSet(snapshot.Memory.Concerns, []string{"phase=IDLE runtime=idle", "concern with [redacted]"}) {
				t.Fatalf("memory blockers/concerns = %+v/%+v, want redacted path-safe evidence", snapshot.Memory.Blockers, snapshot.Memory.Concerns)
			}
			regions := semanticRegionsByName(t, snapshot)
			memory := strings.Join(regions["memory"].Items, "\n")
			if !containsAll(memory, []string{"source: state.current-session-snapshot", "session_id: current", "transcript_turns: 2", "queued_count: 2", "diagnostics: 1", "blocker: interrupt pending", "concern: phase=IDLE runtime=idle", "app-owned", "display-only"}) {
				t.Fatalf("memory semantic region = %v, want machine-readable resumed memory", regions["memory"].Items)
			}
			runtimeStatus := strings.Join(regions["runtime_status"].Items, "\n")
			if !containsAll(runtimeStatus, []string{"status: idle [redacted]", "status source: runtime.dispatch [redacted]", "detail: resumed current session [redacted]", "result: remembered result [redacted]", "display-only"}) {
				t.Fatalf("runtime semantic region = %v, want redacted status context", regions["runtime_status"].Items)
			}
		})
	}
}

func runMemoryFixtureSizes() []fixtureSize {
	return []fixtureSize{{Name: "120x60", Width: 120, Height: 60}}
}

func loadRunMemoryFixture(t *testing.T) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.Scenario = "run-memory"
	state.RuntimeStatus = "idle"
	state.StatusSource = "noninteractive.run"
	state.StatusDetail = "read-only run flagged"
	state.RuntimeResult = "Read-only run flagged: inspected 2 file(s), ran 2 check(s)."
	state.MemorySource = "state.current-session-snapshot"
	state.MemorySessionID = "current"
	state.Transcript = []TranscriptTurn{
		{UserText: "explain the repo"},
		{AssistantText: "Read-only run flagged: inspected 2 file(s), ran 2 check(s)."},
	}
	state.RunMemory = &RunMemoryView{
		Mode:   "non_interactive_read_only",
		Prompt: "explain the repo",
		Status: "flagged",
		InspectedFiles: []RunMemoryFileView{
			{Path: "README.md", Status: "completed", LineStart: 1, LineEnd: 3, SourceRef: "README.md:1-3"},
			{Path: "ROADMAP.md", Status: "completed", LineStart: 1, LineEnd: 3, SourceRef: "ROADMAP.md:1-3"},
		},
		Commands: []RunMemoryCommandView{
			{Command: "git status --short --branch", Status: "completed", ExitCode: 0, Summary: "## main"},
			{Command: "git diff --stat", Status: "completed", ExitCode: 0, Summary: "no output"},
		},
		Blockers:      []string{},
		Caveats:       []string{"deterministic read-only run; provider model execution deferred"},
		SourceRefs:    []string{"README.md:1-3", "ROADMAP.md:1-3", "git status --short --branch", "git diff --stat"},
		StoredSession: true,
		StoredHistory: true,
	}
	state = ApplyHistoryView(state, runMemoryHistoryItems(), 4, true)
	state.Scenario = "run-memory"
	return loadRenderFixture(t, state.Scenario, state)
}

func runMemoryHistoryItems() []HistoryItem {
	return []HistoryItem{
		{
			EventID:     "noninteractive-run-1",
			RunID:       "noninteractive-run",
			SessionID:   "current",
			Kind:        "prompt",
			Source:      "user",
			Provenance:  "run.prompt",
			DisplayText: "noninteractive run prompt explain the repo",
		},
		{
			EventID:     "noninteractive-run-2",
			RunID:       "noninteractive-run",
			SessionID:   "current",
			Kind:        "response",
			Source:      "noninteractive.run",
			Provenance:  "run.response",
			DisplayText: "Read-only run flagged: inspected 2 file(s), ran 2 check(s).",
		},
		{
			EventID:     "noninteractive-run-3",
			RunID:       "noninteractive-run",
			SessionID:   "current",
			Kind:        "runtime",
			Source:      "noninteractive.run",
			Provenance:  "run.complete",
			DisplayText: "noninteractive run flagged inspected=2 commands=2",
		},
		{
			EventID:     "noninteractive-run-4",
			RunID:       "noninteractive-run",
			SessionID:   "current",
			Kind:        "command",
			Source:      "noninteractive.run",
			Provenance:  "run.check",
			DisplayText: "check git status --short --branch completed",
		},
		{
			EventID:     "noninteractive-run-5",
			RunID:       "noninteractive-run",
			SessionID:   "current",
			Kind:        "command",
			Source:      "noninteractive.run",
			Provenance:  "run.check",
			DisplayText: "check git diff --stat completed",
		},
	}
}

func TestRunMemoryRenderAndSemantic(t *testing.T) {
	t.Parallel()

	fixture := loadRunMemoryFixture(t)
	render := RenderPlain(fixture.State, Size{Width: 120, Height: 60})
	wantRender := []string{
		"Runtime status:",
		"status source: noninteractive.run",
		"Resumed memory:",
		"run mode: non_interactive_read_only",
		"run status: flagged",
		"run prompt: explain the repo",
		"inspected file: README.md status=completed source_ref=README.md:1-3",
		"command run: git status --short --branch status=completed",
		"run caveat: deterministic read-only run; provider model execution deferred",
		"source ref: git diff --stat",
		"history:",
		"entries: 5",
		"selected kind: command",
		"selected text: check git diff --stat completed",
	}
	if !containsAll(render, wantRender) {
		t.Fatalf("run-memory render missing evidence %v:\n%s", wantRender, render)
	}

	snapshot := Semantic(fixture.State, Size{Width: 120, Height: 60})
	if snapshot.Memory == nil || snapshot.Memory.Run == nil {
		t.Fatalf("semantic memory missing run evidence: %+v", snapshot.Memory)
	}
	run := snapshot.Memory.Run
	if run.Mode != "non_interactive_read_only" || run.Prompt != "explain the repo" || run.Status != "flagged" || !run.StoredSession || !run.StoredHistory {
		t.Fatalf("semantic run memory = %+v", run)
	}
	if len(run.InspectedFiles) != 2 || run.InspectedFiles[0].Path != "README.md" || len(run.CommandsRun) != 2 || run.CommandsRun[1].Command != "git diff --stat" || !sameStringSet(run.SourceRefs, []string{"README.md:1-3", "ROADMAP.md:1-3", "git status --short --branch", "git diff --stat"}) {
		t.Fatalf("semantic run evidence = %+v", run)
	}
	if snapshot.History == nil || !snapshot.History.ReadOnly || snapshot.History.Count != 5 || snapshot.History.SelectedID != "noninteractive-run-5" {
		t.Fatalf("semantic history = %+v", snapshot.History)
	}
	regions := semanticRegionsByName(t, snapshot)
	memory := strings.Join(regions["memory"].Items, "\n")
	if !containsAll(memory, []string{"run_mode: non_interactive_read_only", "run_status: flagged", "run_prompt: explain the repo", "stored_session: true", "stored_history: true", "inspected_file: README.md status=completed source_ref=README.md:1-3", "command_run: git diff --stat status=completed", "run_caveat: deterministic read-only run; provider model execution deferred", "source_ref: git status --short --branch", "app-owned", "display-only"}) {
		t.Fatalf("memory semantic region = %v, want run memory evidence", regions["memory"].Items)
	}
	history := strings.Join(regions["history"].Items, "\n")
	if !containsAll(history, []string{"read_only: true", "selected_id: noninteractive-run-5", "item: noninteractive-run current noninteractive-run-1 prompt noninteractive run prompt explain the repo selected: false", "item: noninteractive-run current noninteractive-run-5 command check git diff --stat completed selected: true", "app-owned", "display-only"}) {
		t.Fatalf("history semantic region = %v, want non-interactive run event evidence", regions["history"].Items)
	}
}

func TestRunMemoryFixtureSnapshots(t *testing.T) {
	fixture := loadRunMemoryFixture(t)
	assertFixtureSizes(t, fixture, runMemoryFixtureSizes())
	for _, renderCase := range fixture.TextCases() {
		renderCase := renderCase
		t.Run(renderCase.name, func(t *testing.T) {
			got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
			assertTextSnapshot(t, fixture, renderCase.file, got)
			plain := stripANSI(got)
			if !containsAll(plain, []string{"run mode: non_interactive_read_only", "inspected file: README.md", "history:", "selected text: check git diff --stat completed"}) {
				t.Fatalf("run-memory fixture missing visible evidence:\n%s", plain)
			}
		})
	}
	for _, semanticCase := range fixture.SemanticCases() {
		semanticCase := semanticCase
		t.Run(semanticCase.name, func(t *testing.T) {
			got := RenderSemanticJSON(fixture.State, semanticCase.size)
			assertSemanticSnapshot(t, fixture, semanticCase.file, got)
			var snapshot SemanticSnapshot
			if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
				t.Fatalf("unmarshal semantic snapshot: %v", err)
			}
			if snapshot.Memory == nil || snapshot.Memory.Run == nil || snapshot.History == nil {
				t.Fatalf("semantic run-memory fixture missing memory/history: %+v", snapshot)
			}
		})
	}
}

func loadWriteRunMemoryFixture(t *testing.T) renderFixture {
	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.Scenario = "write-run-memory"
	state.RuntimeStatus = "idle"
	state.StatusSource = "noninteractive.run"
	state.StatusDetail = "write run flagged"
	state.RuntimeResult = "Write run flagged: changed 1 file(s), ran 2 check(s)."
	state.MemorySource = "state.current-session-snapshot"
	state.MemorySessionID = "current"
	state.Transcript = []TranscriptTurn{
		{UserText: "create a note"},
		{AssistantText: "Write run flagged: changed 1 file(s), ran 2 check(s)."},
	}
	state.RunMemory = &RunMemoryView{
		Mode:   "non_interactive_write",
		Prompt: "create a note",
		Status: "flagged",
		InspectedFiles: []RunMemoryFileView{
			{Path: "README.md", Status: "completed", LineStart: 1, LineEnd: 3, SourceRef: "README.md:1-3"},
		},
		Commands: []RunMemoryCommandView{
			{Command: "git status --short --branch", Status: "completed", ExitCode: 0, Summary: "## main"},
			{Command: "git diff --stat", Status: "completed", ExitCode: 0, Summary: "docs/aila-run-output.md | 5 +++++"},
		},
		ChangedFiles: []RunMemoryChangedFileView{
			{Path: "docs/aila-run-output.md", Status: "completed", PreviousVersion: "missing", NewVersion: "sha256:write-run", BytesWritten: 120, SourceRef: "docs/aila-run-output.md"},
		},
		Mutation: &RunMemoryMutationView{
			Name:           "write",
			Status:         "completed",
			Path:           "docs/aila-run-output.md",
			ExpectedEffect: "create bounded non-interactive run output",
			BytesWritten:   120,
			Decision: &DecisionView{
				Source:           "autonomy_policy",
				Autonomy:         "write",
				Allowed:          true,
				Automatic:        true,
				ApprovalRequired: false,
				OperationKind:    "file_mutation",
				Name:             "write",
				Target:           "docs/aila-run-output.md",
				ExpectedEffect:   "create bounded non-interactive run output",
				RunID:            "run-write-note",
				Capability:       "noninteractive.run",
			},
		},
		Blockers:      []string{},
		Caveats:       []string{"deterministic write run; provider model execution deferred"},
		SourceRefs:    []string{"README.md:1-3", "docs/aila-run-output.md", "git status --short --branch", "git diff --stat"},
		StoredSession: true,
		StoredHistory: true,
	}
	state = ApplyHistoryView(state, writeRunMemoryHistoryItems(), 5, true)
	state.Scenario = "write-run-memory"
	return loadRenderFixture(t, state.Scenario, state)
}

func writeRunMemoryHistoryItems() []HistoryItem {
	items := runMemoryHistoryItems()
	items[0].DisplayText = "noninteractive run prompt create a note"
	items[1].DisplayText = "Write run flagged: changed 1 file(s), ran 2 check(s)."
	items[2].DisplayText = "noninteractive run flagged inspected=1 commands=2"
	items = append(items, HistoryItem{
		EventID:     "noninteractive-run-6",
		RunID:       "noninteractive-run",
		SessionID:   "current",
		Kind:        "mutation",
		Source:      "noninteractive.run",
		Provenance:  "mutation.result",
		DisplayText: "mutation write completed docs/aila-run-output.md undo delete_created_file",
		Mutation: &HistoryMutationItem{
			Name:                  "write",
			Status:                "completed",
			CommandSource:         "noninteractive.run",
			RequestID:             "run-write-note",
			ChangedPaths:          []string{"docs/aila-run-output.md"},
			RequestedPath:         "docs/aila-run-output.md",
			ExpectedEffect:        "create bounded non-interactive run output",
			PreviousVersion:       "missing",
			NewVersion:            "sha256:write-run",
			BytesWritten:          120,
			ResolvedPathAvailable: true,
			DecisionRunID:         "run-write-note",
			DecisionCapability:    "noninteractive.run",
		},
		Undo: &HistoryUndoItem{
			Available:       true,
			Action:          "delete_created_file",
			Paths:           []string{"docs/aila-run-output.md"},
			PreviousVersion: "missing",
			NewVersion:      "sha256:write-run",
		},
	})
	return items
}

func TestWriteRunMemoryRenderAndSemantic(t *testing.T) {
	t.Parallel()

	fixture := loadWriteRunMemoryFixture(t)
	render := RenderPlain(fixture.State, Size{Width: 120, Height: 60})
	wantRender := []string{
		"run mode: non_interactive_write",
		"run status: flagged",
		"run prompt: create a note",
		"changed file: docs/aila-run-output.md status=completed source_ref=docs/aila-run-output.md",
		"mutation tool: write",
		"mutation status: completed",
		"mutation decision source: autonomy_policy",
		"mutation decision autonomy: write",
		"mutation approval required: false",
		"history:",
		"selected event id: noninteractive-run-6",
	}
	if !containsAll(render, wantRender) {
		t.Fatalf("write-run-memory render missing evidence %v:\n%s", wantRender, render)
	}

	snapshot := Semantic(fixture.State, Size{Width: 120, Height: 60})
	if snapshot.Memory == nil || snapshot.Memory.Run == nil || snapshot.Memory.Run.Mutation == nil {
		t.Fatalf("semantic memory missing write run evidence: %+v", snapshot.Memory)
	}
	run := snapshot.Memory.Run
	if run.Mode != "non_interactive_write" || run.Prompt != "create a note" || len(run.ChangedFiles) != 1 || run.ChangedFiles[0].Path != "docs/aila-run-output.md" {
		t.Fatalf("semantic write run memory = %+v", run)
	}
	if run.Mutation.Name != "write" || run.Mutation.Status != "completed" || run.Mutation.Decision == nil || run.Mutation.Decision.Autonomy != "write" || !run.Mutation.Decision.Allowed || run.Mutation.Decision.ApprovalRequired {
		t.Fatalf("semantic write mutation = %+v", run.Mutation)
	}
	if snapshot.History == nil || snapshot.History.SelectedID != "noninteractive-run-6" {
		t.Fatalf("semantic write history = %+v", snapshot.History)
	}
	selectedMutation := false
	for _, item := range snapshot.History.Items {
		if item.EventID == "noninteractive-run-6" && item.Selected && item.Kind == "mutation" && item.Mutation != nil && item.Undo != nil {
			selectedMutation = true
		}
	}
	if !selectedMutation {
		t.Fatalf("semantic write history items = %+v", snapshot.History.Items)
	}
	regions := semanticRegionsByName(t, snapshot)
	memory := strings.Join(regions["memory"].Items, "\n")
	if !containsAll(memory, []string{"run_mode: non_interactive_write", "changed_file: docs/aila-run-output.md status=completed source_ref=docs/aila-run-output.md", "mutation_tool: write", "mutation_status: completed", "mutation_decision_source: autonomy_policy", "mutation_decision_autonomy: write", "mutation_approval_required: false", "app-owned", "display-only"}) {
		t.Fatalf("memory semantic region = %v, want write run evidence", regions["memory"].Items)
	}
	history := strings.Join(regions["history"].Items, "\n")
	if !containsAll(history, []string{"selected_id: noninteractive-run-6", "item: noninteractive-run current noninteractive-run-6 mutation mutation write completed docs/aila-run-output.md undo delete_created_file selected: true", "item_mutation: noninteractive-run-6 write completed", "item_changed_paths: docs/aila-run-output.md", "item_undo_action: delete_created_file", "display-only"}) {
		t.Fatalf("history semantic region = %v, want write mutation history evidence", regions["history"].Items)
	}
}

func TestWriteRunMemoryFixtureSnapshots(t *testing.T) {
	fixture := loadWriteRunMemoryFixture(t)
	assertFixtureSizes(t, fixture, runMemoryFixtureSizes())
	for _, renderCase := range fixture.TextCases() {
		renderCase := renderCase
		t.Run(renderCase.name, func(t *testing.T) {
			got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
			assertTextSnapshot(t, fixture, renderCase.file, got)
			plain := stripANSI(got)
			if !containsAll(plain, []string{"run mode: non_interactive_write", "changed file: docs/aila-run-output.md", "mutation tool: write", "selected event id: noninteractive-run-6"}) {
				t.Fatalf("write-run-memory fixture missing visible evidence:\n%s", plain)
			}
		})
	}
	for _, semanticCase := range fixture.SemanticCases() {
		semanticCase := semanticCase
		t.Run(semanticCase.name, func(t *testing.T) {
			got := RenderSemanticJSON(fixture.State, semanticCase.size)
			assertSemanticSnapshot(t, fixture, semanticCase.file, got)
			var snapshot SemanticSnapshot
			if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
				t.Fatalf("unmarshal semantic snapshot: %v", err)
			}
			if snapshot.Memory == nil || snapshot.Memory.Run == nil || snapshot.Memory.Run.Mutation == nil || snapshot.History == nil {
				t.Fatalf("semantic write-run-memory fixture missing memory/history: %+v", snapshot)
			}
		})
	}
}

func viewStateFromRuntimeModel(scenario string, model runtime.Model) ViewState {
	state := IdleEmptyState()
	state.Phase = "PLAN"
	state.PhaseSource = "workflow.fixture"
	state.Scenario = scenario
	state.RuntimeStatus = string(model.Status)
	state.StatusSource = "runtime.fixture"
	state.StatusDetail = "fake in-memory runtime loop"
	state.RuntimeActive = model.Status == runtime.StatusActive
	state.RuntimeResult = model.Result
	state.Transcript = transcriptTurnsFromRuntime(model.Transcript)
	return state
}

func transcriptTurnsFromRuntime(entries []runtime.TranscriptEntry) []TranscriptTurn {
	var turns []TranscriptTurn
	for _, entry := range entries {
		switch entry.Kind {
		case "prompt":
			turns = append(turns, TranscriptTurn{UserText: entry.Text})
		case "result", "failure":
			if len(turns) == 0 || turns[len(turns)-1].AssistantText != "" {
				turns = append(turns, TranscriptTurn{})
			}
			turns[len(turns)-1].AssistantText = entry.Text
		}
	}
	return turns
}

func TestM12RuntimeStatusFixturesDistinguishPhaseFromRuntime(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		status     string
		active     bool
		wantRender []string
	}{
		{name: "runtime-idle", status: "idle", wantRender: []string{"Stage PLAN | Runtime idle", "status: idle", "active: false"}},
		{name: "runtime-active", status: "active", active: true, wantRender: []string{"Stage PLAN | Runtime active", "status: active", "active: true", "user: explain runtime status"}},
		{name: "runtime-result", status: "idle", wantRender: []string{"Stage PLAN | Runtime idle", "status: idle", "active: false", "result: Fake Aila response: explain runtime status", "assistant: Fake Aila response: explain runtime status"}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := loadRuntimeStatusFixture(t, tc.name)
			assertFixtureSizes(t, fixture, []fixtureSize{{Name: "80x24", Width: 80, Height: 24}})
			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := renderCase.render(fixture.State, renderCase.size)
					assertTextSnapshot(t, fixture, renderCase.file, got)
					if !containsAll(got, tc.wantRender) {
						t.Fatalf("%s render missing runtime evidence %v:\n%s", tc.name, tc.wantRender, got)
					}
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
					if snapshot.Session.Phase != "PLAN" || snapshot.Session.PhaseSource != "workflow.fixture" {
						t.Fatalf("phase = %q from %q, want injected workflow phase", snapshot.Session.Phase, snapshot.Session.PhaseSource)
					}
					if snapshot.Session.RuntimeStatus != tc.status || snapshot.Session.StatusSource != "runtime.fixture" || snapshot.Session.Active != tc.active {
						t.Fatalf("runtime session = %+v, want status %q active %v from runtime fixture", snapshot.Session, tc.status, tc.active)
					}
					regions := semanticRegionsByName(t, snapshot)
					phase := strings.Join(regions["phase"].Items, "\n")
					status := strings.Join(regions["runtime_status"].Items, "\n")
					if !containsAll(phase, []string{"PLAN", "display-only"}) || containsAny(phase, []string{"active", "idle", "result"}) {
						t.Fatalf("phase region = %v, want workflow phase only", regions["phase"].Items)
					}
					if !containsAll(status, []string{"status: " + tc.status, "status source: runtime.fixture", "active: " + boolLabel(tc.active), "display-only"}) || contains(status, "phase") {
						t.Fatalf("runtime status region = %v, want injected runtime status only", regions["runtime_status"].Items)
					}
				})
			}
		})
	}
}

func TestQueuedMessageFixtureSnapshots(t *testing.T) {
	t.Parallel()

	fixture := loadQueuedMessageFixture(t)
	if fixture.Kind != "static_shell" {
		t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
	}
	if fixture.TerminalBehavior != "bubbletea_static" {
		t.Fatalf("terminal behavior = %q, want bubbletea_static", fixture.TerminalBehavior)
	}
	assertFixtureSizes(t, fixture, []fixtureSize{{Name: "80x24", Width: 80, Height: 24}})

	wantRender := []string{
		"Stage PLAN | Runtime active",
		"status: active",
		"active: true",
		"user: active fake work",
		"Queued input:",
		"queued messages: 2",
		"default action: send after current turn",
		"action status: presentation-only; not executed by the TUI",
		"queued: refine the tests",
		"queued: explain the diff",
	}
	for _, renderCase := range fixture.TextCases() {
		renderCase := renderCase
		t.Run(renderCase.name, func(t *testing.T) {
			t.Parallel()

			got := renderCase.render(fixture.State, renderCase.size)
			assertTextSnapshot(t, fixture, renderCase.file, got)
			if !containsAll(got, wantRender) {
				t.Fatalf("queued-message render missing active work, queued intent, or default behavior %v:\n%s", wantRender, got)
			}
			if containsAny(got, []string{"interrupt", "steer", "cancel"}) {
				t.Fatalf("queued-message render implies non-default execution choices:\n%s", got)
			}
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
			if snapshot.Session.QueuedMessages != 2 || !snapshot.Session.Active {
				t.Fatalf("session = %+v, want active queued fixture state", snapshot.Session)
			}
			regions := semanticRegionsByName(t, snapshot)
			chat := strings.Join(regions["chat"].Items, "\n")
			queue := strings.Join(regions["queue"].Items, "\n")
			if !containsAll(chat, []string{"user: active fake work"}) || !containsAll(queue, []string{
				"queued messages: 2",
				"default action: send after current turn",
				"presentation-only",
				"executed: false",
				"queued: refine the tests",
				"queued: explain the diff",
			}) {
				t.Fatalf("queued semantic regions missing active work or queued intent: chat=%v queue=%v", regions["chat"].Items, regions["queue"].Items)
			}
			assertQueuedDefaultAction(t, snapshot)
		})
	}
}

func TestInterruptFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		status     string
		active     bool
		wantRender []string
		wantRegion []string
	}{
		{
			status: "canceling",
			active: true,
			wantRender: []string{
				"Stage BUILD | Runtime canceling",
				"Runtime status:",
				"status: canceling",
				"active: true",
				"interrupt state:",
				"interrupt status: canceling",
				"outcome: pending",
				"lower-layer cancellation executed: false",
				"user: active fake work",
			},
			wantRegion: []string{
				"state: canceling",
				"outcome: pending",
				"lower_layer_cancellation_executed: false",
				"display-only",
			},
		},
		{
			status: "canceled",
			active: false,
			wantRender: []string{
				"Stage BUILD | Runtime canceled",
				"Runtime status:",
				"status: canceled",
				"active: false",
				"result: fake work canceled",
				"interrupt state:",
				"interrupt status: canceled",
				"outcome: fake work canceled",
				"lower-layer cancellation executed: false",
				"user: active fake work",
			},
			wantRegion: []string{
				"state: canceled",
				"outcome: fake work canceled",
				"lower_layer_cancellation_executed: false",
				"display-only",
			},
		},
	} {
		tc := tc
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()

			fixture := loadInterruptFixture(t, tc.status)
			if fixture.Kind != "static_shell" {
				t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
			}
			if fixture.TerminalBehavior != "bubbletea_static" {
				t.Fatalf("terminal behavior = %q, want bubbletea_static", fixture.TerminalBehavior)
			}
			assertFixtureSizes(t, fixture, []fixtureSize{{Name: "80x24", Width: 80, Height: 24}})

			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := renderCase.render(fixture.State, renderCase.size)
					assertTextSnapshot(t, fixture, renderCase.file, got)
					if !containsAll(got, tc.wantRender) {
						t.Fatalf("%s render missing interrupt fixture evidence %v:\n%s", fixture.Name, tc.wantRender, got)
					}
					if containsAny(got, []string{"shell canceled", "model canceled", "tool canceled", "runtime canceled by TUI"}) {
						t.Fatalf("%s render implies real lower-layer cancellation:\n%s", fixture.Name, got)
					}
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
					if snapshot.Session.RuntimeStatus != tc.status || snapshot.Session.Active != tc.active {
						t.Fatalf("session = %+v, want status %s active %v", snapshot.Session, tc.status, tc.active)
					}
					if snapshot.Interrupt == nil || snapshot.Interrupt.State != tc.status || snapshot.Interrupt.LowerLayerCancellationExecuted {
						t.Fatalf("interrupt = %+v, want status %s without lower-layer cancellation", snapshot.Interrupt, tc.status)
					}
					regions := semanticRegionsByName(t, snapshot)
					chat := strings.Join(regions["chat"].Items, "\n")
					interrupt := strings.Join(regions["interrupt"].Items, "\n")
					if !contains(chat, "user: active fake work") || !containsAll(interrupt, tc.wantRegion) {
						t.Fatalf("semantic regions missing interrupt evidence: chat=%v interrupt=%v", regions["chat"].Items, regions["interrupt"].Items)
					}
				})
			}
		})
	}
}

func TestM15ProjectStoreStatusFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status string
		detail string
	}{
		{name: "store-initialized", status: "initialized", detail: "project store ready"},
		{name: "store-uninitialized", status: "uninitialized", detail: "project store not opened"},
		{name: "store-degraded", status: "degraded", detail: "create store directory"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := loadProjectStoreFixture(t, tc.name)
			if fixture.Kind != "static_shell" {
				t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
			}
			assertFixtureSizes(t, fixture, []fixtureSize{{Name: "80x24", Width: 80, Height: 24}})

			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := renderCase.render(fixture.State, renderCase.size)
					assertTextSnapshot(t, fixture, renderCase.file, got)
					if !containsAll(got, []string{"project store: " + tc.status, tc.detail, "primary model: opencode-go/deepseek-v4-pro:high"}) {
						t.Fatalf("%s render missing project store evidence:\n%s", fixture.Name, got)
					}
					assertNoPathLeak(t, got)
				})
			}

			for _, semanticCase := range fixture.SemanticCases() {
				semanticCase := semanticCase
				t.Run(semanticCase.name, func(t *testing.T) {
					t.Parallel()

					got := RenderSemanticJSON(fixture.State, semanticCase.size)
					assertSemanticSnapshot(t, fixture, semanticCase.file, got)
					assertNoPathLeak(t, got)
					var snapshot SemanticSnapshot
					if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
						t.Fatalf("unmarshal semantic snapshot: %v", err)
					}
					if snapshot.Session.ProjectStoreStatus != tc.status || snapshot.Session.ProjectStoreSource != "state.open" || snapshot.Session.ProjectStoreDetail != tc.detail {
						t.Fatalf("session store status = %+v, want %s from state.open", snapshot.Session, tc.status)
					}
					regions := semanticRegionsByName(t, snapshot)
					store := strings.Join(regions["project_store"].Items, "\n")
					if !containsAll(store, []string{"status: " + tc.status, "source: state.open", "detail: " + tc.detail, "app-owned"}) {
						t.Fatalf("project_store region = %v, want path-safe app-owned status", regions["project_store"].Items)
					}
				})
			}
		})
	}
}

func TestM15ADiagnosticFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		severity      string
		source        string
		recovery      string
		artifact      string
		inputNeeded   bool
		wantRender    []string
		forbiddenText []string
	}{
		{
			name:        "diagnostic-ready",
			severity:    "warning",
			source:      "runtime.fixture",
			recovery:    "inspect",
			artifact:    "runtime",
			inputNeeded: false,
			wantRender: []string{
				"Diagnostics:",
				"severity: warning",
				"source: runtime.fixture",
				"affected artifact: runtime",
				"recovery action: inspect",
				"user input needed: false",
				"message: runtime cancellation was recorded as diagnostic state",
			},
			forbiddenText: []string{"state.open", "project store:", "repair executed", "storage owner"},
		},
		{
			name:        "corrupt-state-recovery",
			severity:    "error",
			source:      "state.open",
			recovery:    "manual-repair",
			artifact:    "project-metadata",
			inputNeeded: true,
			wantRender: []string{
				"project store: recovery-needed - project metadata needs manual review",
				"severity: error",
				"source: state.open",
				"affected artifact: project-metadata",
				"recovery action: manual-repair",
				"user input needed: true",
				"metadata unreadable; inspect before reinitialize",
			},
			forbiddenText: []string{"repair executed", "destructive repair", "provider fallback", "session resume"},
		},
		{
			name:        "graceful-shutdown",
			severity:    "info",
			source:      "signal.shutdown",
			recovery:    "none",
			artifact:    "runtime",
			inputNeeded: false,
			wantRender: []string{
				"Stage IDLE | Runtime canceled",
				"severity: info",
				"source: signal.shutdown",
				"affected artifact: runtime",
				"recovery action: none",
				"user input needed: false",
				"graceful shutdown completed after cancellation",
			},
			forbiddenText: []string{"session resume", "replay", "undo", "provider fallback", "repair executed", "destructive repair"},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := loadDiagnosticFixture(t, tc.name)
			if fixture.Kind != "static_shell" {
				t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
			}
			assertFixtureSizes(t, fixture, []fixtureSize{{Name: "80x24", Width: 80, Height: 24}})

			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := renderCase.render(fixture.State, renderCase.size)
					assertTextSnapshot(t, fixture, renderCase.file, got)
					if !containsAll(got, tc.wantRender) {
						t.Fatalf("%s render missing diagnostic evidence %v:\n%s", fixture.Name, tc.wantRender, got)
					}
					assertNoDiagnosticLeak(t, got)
					if containsAny(got, tc.forbiddenText) {
						t.Fatalf("%s render implies forbidden behavior:\n%s", fixture.Name, got)
					}
				})
			}

			for _, semanticCase := range fixture.SemanticCases() {
				semanticCase := semanticCase
				t.Run(semanticCase.name, func(t *testing.T) {
					t.Parallel()

					got := RenderSemanticJSON(fixture.State, semanticCase.size)
					assertSemanticSnapshot(t, fixture, semanticCase.file, got)
					assertNoDiagnosticLeak(t, got)
					var snapshot SemanticSnapshot
					if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
						t.Fatalf("unmarshal semantic snapshot: %v", err)
					}
					assertSemanticDiagnostic(t, snapshot, SemanticDiagnostic{
						Severity:         tc.severity,
						Source:           tc.source,
						RecoveryAction:   tc.recovery,
						AffectedArtifact: tc.artifact,
						UserInputNeeded:  tc.inputNeeded,
						BoundedMessage:   fixture.State.Diagnostics[0].BoundedMessage,
					})
					if containsAny(got, tc.forbiddenText) {
						t.Fatalf("%s semantic snapshot implies forbidden behavior:\n%s", fixture.Name, got)
					}
				})
			}
		})
	}
}

func TestM15ADiagnosticSupportLeavesExistingFixtureEvidenceStable(t *testing.T) {
	t.Parallel()

	for _, fixture := range []renderFixture{
		loadQueuedMessageFixture(t),
		loadInterruptFixture(t, "canceling"),
		loadInterruptFixture(t, "canceled"),
		loadProjectStoreFixture(t, "store-initialized"),
		loadProjectStoreFixture(t, "store-uninitialized"),
		loadProjectStoreFixture(t, "store-degraded"),
	} {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			render := RenderPlain(fixture.State, Size{Width: 80, Height: 24})
			assertTextSnapshot(t, fixture, "plain-80x24.txt", render)
			semantic := RenderSemanticJSON(fixture.State, Size{Width: 80, Height: 24})
			assertSemanticSnapshot(t, fixture, "semantic-80x24.json", semantic)
			if containsAny(render+semantic, []string{"Diagnostics:", "recovery action:", "\"diagnostics\""}) {
				t.Fatalf("%s existing fixture gained diagnostic metadata unexpectedly", fixture.Name)
			}
		})
	}
}

func TestM16MemorySupportLeavesExistingFixtureEvidenceStable(t *testing.T) {
	t.Parallel()

	for _, fixture := range []renderFixture{
		loadQueuedMessageFixture(t),
		loadInterruptFixture(t, "canceling"),
		loadInterruptFixture(t, "canceled"),
		loadProjectStoreFixture(t, "store-initialized"),
		loadProjectStoreFixture(t, "store-uninitialized"),
		loadProjectStoreFixture(t, "store-degraded"),
		loadDiagnosticFixture(t, "diagnostic-ready"),
	} {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			render := RenderPlain(fixture.State, Size{Width: 80, Height: 24})
			assertTextSnapshot(t, fixture, "plain-80x24.txt", render)
			semantic := RenderSemanticJSON(fixture.State, Size{Width: 80, Height: 24})
			assertSemanticSnapshot(t, fixture, "semantic-80x24.json", semantic)
			if containsAny(render+semantic, []string{"Resumed memory:", "session id:", "\"memory\"", "state.current-session-snapshot"}) {
				t.Fatalf("%s existing fixture gained memory metadata unexpectedly", fixture.Name)
			}
		})
	}
}

func assertSemanticDiagnostic(t *testing.T, snapshot SemanticSnapshot, want SemanticDiagnostic) {
	t.Helper()

	if len(snapshot.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want one diagnostic", snapshot.Diagnostics)
	}
	if snapshot.Diagnostics[0] != want {
		t.Fatalf("diagnostic = %+v, want %+v", snapshot.Diagnostics[0], want)
	}
	regions := semanticRegionsByName(t, snapshot)
	diagnostic, ok := regions["diagnostics"]
	if !ok || !diagnostic.Visible {
		t.Fatalf("diagnostics region = %+v, want visible", diagnostic)
	}
	items := strings.Join(diagnostic.Items, "\n")
	if !containsAll(items, []string{
		"severity: " + want.Severity,
		"source: " + want.Source,
		"affected_artifact: " + want.AffectedArtifact,
		"recovery_action: " + want.RecoveryAction,
		"user_input_needed: " + boolLabel(want.UserInputNeeded),
		"bounded_message: " + want.BoundedMessage,
		"app-owned",
		"display-only",
	}) {
		t.Fatalf("diagnostic semantic region = %+v, want stable machine-readable fields", diagnostic.Items)
	}
}

func assertNoDiagnosticLeak(t *testing.T, text string) {
	t.Helper()
	assertNoPathLeak(t, text)
	if containsAny(text, []string{"secret", "token=", "api_key", "authorization", "Bearer "}) {
		t.Fatalf("diagnostic fixture leaked secret-like text:\n%s", text)
	}
}

func assertNoMemoryLeak(t *testing.T, text string) {
	t.Helper()
	assertNoPathLeak(t, text)
	if containsAny(text, []string{"\x1b", "secret", "token=", "api_key", "password=", "authorization", "Bearer "}) {
		t.Fatalf("memory fixture leaked control or secret-like text:\n%s", text)
	}
}

func assertNoHistoryLeak(t *testing.T, text string) {
	t.Helper()
	assertNoPathLeak(t, text)
	if containsAny(text, []string{"\x1b", "secret", "token=", "api_key", "password=", "authorization", "Bearer "}) {
		t.Fatalf("history fixture leaked control or secret-like text:\n%s", text)
	}
}

func assertQueuedDefaultAction(t *testing.T, snapshot SemanticSnapshot) {
	t.Helper()

	for _, action := range snapshot.Actions {
		if action.Name != "queue_after_current_turn" {
			continue
		}
		if action.Input != "enter" || !action.Default || !action.PresentationOnly || action.Executed {
			t.Fatalf("queue action = %+v, want default presentation-only non-executed action", action)
		}
		return
	}
	t.Fatalf("actions = %+v, want queue_after_current_turn", snapshot.Actions)
}

func assertNoPathLeak(t *testing.T, text string) {
	t.Helper()
	if containsAny(text, []string{"/tmp", "/home/", "/etc/", "/var/", "$HOME", "${HOME}", "$XDG_", "\\", ".aila", "project.toml", "artifacts/", "indexes/"}) {
		t.Fatalf("rendered project store status leaked path-like text:\n%s", text)
	}
}

func TestM11WaitingStatusFixtureDistinguishesPhaseFromRuntimeStatus(t *testing.T) {
	t.Parallel()

	fixture := loadWaitingStatusFixture(t)
	if fixture.Kind != "static_shell" {
		t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
	}
	assertFixtureSizes(t, fixture, []fixtureSize{{Name: "80x24", Width: 80, Height: 24}})

	for _, renderCase := range fixture.TextCases() {
		renderCase := renderCase
		t.Run(renderCase.name, func(t *testing.T) {
			t.Parallel()

			got := renderCase.render(fixture.State, renderCase.size)
			assertTextSnapshot(t, fixture, renderCase.file, got)
			if !containsAll(got, []string{
				"Stage PLAN | Runtime waiting",
				"Runtime status:",
				"status: waiting",
				"status source: runtime.fixture",
				"detail: successor blocked by injected blocker",
			}) {
				t.Fatalf("waiting fixture render does not expose injected status separately from phase:\n%s", got)
			}
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
			if snapshot.Session.Phase != "PLAN" || snapshot.Session.PhaseSource != "workflow.fixture" {
				t.Fatalf("phase = %q from %q, want injected workflow phase", snapshot.Session.Phase, snapshot.Session.PhaseSource)
			}
			if snapshot.Session.RuntimeStatus != "waiting" || snapshot.Session.StatusSource != "runtime.fixture" || snapshot.Session.StatusDetail != "successor blocked by injected blocker" {
				t.Fatalf("runtime status = %+v, want injected status data separate from phase", snapshot.Session)
			}
			regions := semanticRegionsByName(t, snapshot)
			phase := strings.Join(regions["phase"].Items, "\n")
			status := strings.Join(regions["runtime_status"].Items, "\n")
			if !containsAll(phase, []string{"PLAN", "display-only"}) || contains(phase, "waiting") {
				t.Fatalf("phase region = %v, want workflow phase only", regions["phase"].Items)
			}
			if !containsAll(status, []string{"status: waiting", "status source: runtime.fixture", "display-only"}) || contains(status, "phase") {
				t.Fatalf("runtime status region = %v, want status data only", regions["runtime_status"].Items)
			}
		})
	}
}

func TestM8DisplayFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		want []string
	}{
		{name: "model-display", want: []string{"primary model: opencode-go/deepseek-v4-pro:high", "utility model: opencode-go/deepseek-v4-flash:max", "autonomy: yolo (display-only)"}},
		{name: "autonomy-display", want: []string{"primary model: opencode-go/deepseek-v4-pro:high", "utility model: opencode-go/deepseek-v4-flash:max", "autonomy: read (display-only)"}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := loadDisplayFixture(t, tc.name)
			assertFixtureSizes(t, fixture, m6FixtureSizes())
			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := renderCase.render(fixture.State, renderCase.size)
					assertTextSnapshot(t, fixture, renderCase.file, got)
					if !containsAll(got, tc.want) {
						t.Fatalf("%s %s missing display labels %v:\n%s", tc.name, renderCase.name, tc.want, got)
					}
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
					assertSemanticContract(t, tc.name, semanticCase.size, snapshot)
					assertDisplaySemanticLabels(t, snapshot, fixture.State)
				})
			}
		})
	}
}

func assertDisplaySemanticLabels(t *testing.T, snapshot SemanticSnapshot, state ViewState) {
	t.Helper()

	if snapshot.Session.PrimaryModel != state.PrimaryModel {
		t.Fatalf("semantic primary model = %q, want %q", snapshot.Session.PrimaryModel, state.PrimaryModel)
	}
	if snapshot.Session.UtilityModel != state.UtilityModel {
		t.Fatalf("semantic utility model = %q, want %q", snapshot.Session.UtilityModel, state.UtilityModel)
	}
	if snapshot.Session.Autonomy != state.Autonomy {
		t.Fatalf("semantic autonomy = %q, want %q", snapshot.Session.Autonomy, state.Autonomy)
	}

	regions := semanticRegionsByName(t, snapshot)
	model := strings.Join(regions["model"].Items, "\n")
	if !containsAll(model, []string{"primary: " + state.PrimaryModel, "utility: " + state.UtilityModel, "autonomy: " + state.Autonomy}) {
		t.Fatalf("model semantic items = %v, want configured display labels", regions["model"].Items)
	}
	labels := strings.Join(regions["display_labels"].Items, "\n")
	if !containsAll(labels, normalizedDisplayLabels(state)) {
		t.Fatalf("display label semantic items = %v, want rendered display labels", regions["display_labels"].Items)
	}
	if snapshot.Layout.RightRailVisible {
		rail := strings.Join(regions["right_rail"].Items, "\n")
		if !containsAll(rail, normalizedDisplayLabels(state)) {
			t.Fatalf("right rail semantic items = %v, want configured display labels", regions["right_rail"].Items)
		}
	}
}

func TestM8SemanticDisplayLabelsMatchRenderedLabels(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"model-display", "autonomy-display"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadDisplayFixture(t, name)
			for _, size := range []Size{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 120, Height: 32}, {Width: 160, Height: 45}} {
				size := size
				t.Run(sizeString(size), func(t *testing.T) {
					t.Parallel()

					rendered := RenderPlain(fixture.State, size)
					snapshot := Semantic(fixture.State, size)
					regions := semanticRegionsByName(t, snapshot)
					renderedLabels := renderedDisplayLabels(t, rendered)
					semanticLabels := normalizedDisplayLabels(fixture.State)

					if !containsAll(strings.Join(regions["display_labels"].Items, "\n"), renderedLabels) {
						t.Fatalf("semantic display labels do not match rendered labels %v: %+v", renderedLabels, regions["display_labels"].Items)
					}
					if !sameStringSet(renderedLabels, semanticLabels) {
						t.Fatalf("rendered labels = %v, semantic contract labels = %v", renderedLabels, semanticLabels)
					}
					if snapshot.Session.PrimaryModel != fixture.State.PrimaryModel || snapshot.Session.UtilityModel != fixture.State.UtilityModel || snapshot.Session.Autonomy != fixture.State.Autonomy {
						t.Fatalf("session labels are not machine-readable: %+v", snapshot.Session)
					}
				})
			}
		})
	}
}

func TestM8DisplayLabelSnapshotsKeepLayoutHierarchy(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"model-display", "autonomy-display"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadDisplayFixture(t, name)
			for _, size := range []Size{{Width: 80, Height: 24}, {Width: 100, Height: 30}, {Width: 120, Height: 32}, {Width: 160, Height: 45}} {
				size := size
				t.Run(sizeString(size), func(t *testing.T) {
					t.Parallel()

					rendered := RenderPlain(fixture.State, size)
					if lineCount := strings.Count(rendered, "\n") + 1; lineCount != size.Height {
						t.Fatalf("rendered %d lines, want fixed height %d:\n%s", lineCount, size.Height, rendered)
					}
					for _, marker := range []string{"Aila", "Conversation", "Prompt", "git: placeholder | context: placeholder | q quit"} {
						if !strings.Contains(rendered, marker) {
							t.Fatalf("render missing hierarchy marker %q:\n%s", marker, rendered)
						}
					}
					if size.Width < 160 && strings.Contains(rendered, "Session") {
						t.Fatalf("narrow display render exposed right rail:\n%s", rendered)
					}
					if size.Width >= 160 && !strings.Contains(rendered, "Session") {
						t.Fatalf("wide display render lost right rail:\n%s", rendered)
					}
				})
			}
		})
	}
}

func normalizedDisplayLabels(state ViewState) []string {
	return []string{
		"primary model: " + state.PrimaryModel,
		"utility model: " + state.UtilityModel,
		"autonomy: " + state.Autonomy,
	}
}

func renderedDisplayLabels(t *testing.T, rendered string) []string {
	t.Helper()

	var labels []string
	for _, line := range strings.Split(rendered, "\n") {
		line = strings.Trim(line, "| ")
		line = strings.SplitN(line, " |  | ", 2)[0]
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, " (display-only)")
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "primary model: ") || strings.HasPrefix(line, "utility model: ") || strings.HasPrefix(line, "autonomy: ") {
			labels = append(labels, line)
		}
	}
	if len(labels) != 3 {
		t.Fatalf("rendered display labels = %v, want exactly primary, utility, autonomy labels", labels)
	}
	return labels
}

func sameStringSet(first []string, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	want := map[string]bool{}
	for _, item := range second {
		want[item] = true
	}
	for _, item := range first {
		if !want[item] {
			return false
		}
	}
	return true
}

func TestPromptInputUXFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"editor-open", "file-reference-picker", "file-reference-inserted", "pasted-lines"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadPromptInputFixture(t, name)
			if fixture.Kind != "static_shell" || fixture.TerminalBehavior != "bubbletea_static" || fixture.QuitInput != "q" {
				t.Fatalf("prompt input fixture metadata = %+v", fixture)
			}
			assertFixtureSizes(t, fixture, m6FixtureSizes())
			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := renderCase.render(fixture.State, renderCase.size)
					assertTextSnapshot(t, fixture, renderCase.file, got)
					plain := stripANSI(got)
					switch name {
					case "editor-open":
						if !containsAll(plain, []string{"editor:", "source: app.editor", "status: open", "> draft prompt"}) {
							t.Fatalf("editor fixture missing visible evidence:\n%s", plain)
						}
					case "file-reference-picker":
						if !containsAll(plain, []string{"file-reference:", "source: app.file-reference", "files:", "> README.md", "> summarize @"}) {
							t.Fatalf("file-reference picker fixture missing visible evidence:\n%s", plain)
						}
					case "file-reference-inserted":
						if !containsAll(plain, []string{"status: inserted", "docs/guide.md", "> summarize @docs/guide.md"}) {
							t.Fatalf("file-reference inserted fixture missing visible evidence:\n%s", plain)
						}
					case "pasted-lines":
						if !containsAll(plain, []string{"[Pasted lines +4]"}) {
							t.Fatalf("pasted-lines fixture missing summary:\n%s", plain)
						}
					}
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
					regions := semanticRegionsByName(t, snapshot)
					prompt := strings.Join(regions["prompt"].Items, "\n")
					switch name {
					case "file-reference-picker":
						refs := strings.Join(regions["file_reference"].Items, "\n")
						if snapshot.Screen.Focus != "file_reference" || !containsAll(refs, []string{"read_only: true", "app_owned: true", "item: README.md selected=true"}) {
							t.Fatalf("file-reference picker semantics = focus %q region %s", snapshot.Screen.Focus, refs)
						}
					case "file-reference-inserted":
						if !containsAll(prompt, []string{"file_ref: docs/guide.md"}) {
							t.Fatalf("file-reference inserted prompt semantics = %s", prompt)
						}
					case "pasted-lines":
						if !containsAll(prompt, []string{"paste_summary: [Pasted lines +4]", "exact_text_ref: prompt_input", "exact_text_preserved: true"}) {
							t.Fatalf("pasted-lines prompt semantics = %s", prompt)
						}
					}
				})
			}
		})
	}
}

func TestModelAndAutonomySwitchFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"model-switch", "autonomy-switch"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadSwitchFixture(t, name)
			if fixture.Kind != "static_shell" || fixture.TerminalBehavior != "bubbletea_static" || fixture.QuitInput != "q" {
				t.Fatalf("switch fixture metadata = %+v", fixture)
			}
			assertFixtureSizes(t, fixture, m6FixtureSizes())
			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := renderCase.render(fixture.State, renderCase.size)
					assertTextSnapshot(t, fixture, renderCase.file, got)
					plain := stripANSI(got)
					if name == "model-switch" && !containsAll(plain, []string{"model:", "source: app.model", "target: primary_model", "current primary: opencode-go/deepseek-v4-pro:high", "models:"}) {
						t.Fatalf("model switch fixture missing visible evidence:\n%s", plain)
					}
					if name == "autonomy-switch" && !containsAll(plain, []string{"auto:", "source: app.autonomy", "current: read", "levels:", "read status=available current=true"}) {
						t.Fatalf("autonomy switch fixture missing visible evidence:\n%s", plain)
					}
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
					regions := semanticRegionsByName(t, snapshot)
					if name == "model-switch" {
						if snapshot.ModelSwitch == nil || snapshot.Screen.Focus != "model_switch" || snapshot.ModelSwitch.SelectedLabel != "opencode-go/deepseek-v4-pro:high" {
							t.Fatalf("model switch semantic = focus %q switch %+v", snapshot.Screen.Focus, snapshot.ModelSwitch)
						}
						if !containsAll(strings.Join(regions["model_switch"].Items, "\n"), []string{"current_primary: opencode-go/deepseek-v4-pro:high", "status=degraded", "status=unavailable", "app-owned", "display-only"}) {
							t.Fatalf("model switch semantic region = %v", regions["model_switch"].Items)
						}
					}
					if name == "autonomy-switch" {
						if snapshot.AutonomySwitch == nil || snapshot.Screen.Focus != "autonomy_switch" || snapshot.AutonomySwitch.SelectedLevel != "read" || snapshot.Session.Autonomy != "read" {
							t.Fatalf("autonomy switch semantic = focus %q switch %+v session %+v", snapshot.Screen.Focus, snapshot.AutonomySwitch, snapshot.Session)
						}
						if !containsAll(strings.Join(regions["autonomy_switch"].Items, "\n"), []string{"current: read", "selected_level: read", "item: read status=available current=true selected=true", "app-owned", "display-only"}) {
							t.Fatalf("autonomy switch semantic region = %v", regions["autonomy_switch"].Items)
						}
					}
				})
			}
		})
	}
}

func TestSessionCommandFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"fresh-session", "resumed-session", "cleared-session"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadSessionCommandFixture(t, name)
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
					if !containsAll(stripANSI(got), []string{"session:", "source: app.session", "session id: current"}) {
						t.Fatalf("session fixture missing visible session evidence:\n%s", got)
					}
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
					if snapshot.Session.SessionID != "current" || snapshot.Session.MemoryStatus == "" || snapshot.SessionView == nil || snapshot.SessionView.SessionID != "current" {
						t.Fatalf("session semantic missing identity/status: session=%+v view=%+v", snapshot.Session, snapshot.SessionView)
					}
					regions := semanticRegionsByName(t, snapshot)
					session := strings.Join(regions["session"].Items, "\n")
					if !containsAll(session, []string{"source: app.session", "session_id: current", "memory_status:", "app-owned", "display-only"}) {
						t.Fatalf("session semantic region = %v", regions["session"].Items)
					}
				})
			}
		})
	}
}

func TestCommandFixtureSet(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input string
		route policy.CommandRoute
	}{
		{name: "status-command", input: "/status", route: policy.CommandRouteStatus},
		{name: "review-command", input: "/review", route: policy.CommandRouteReview},
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
					assertOrdered(t, got, "route source: policy.command", commandFixtureMarker(string(tc.route)))
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
	if snapshot.Command.Executed {
		t.Fatalf("command metadata implies execution: %+v", *snapshot.Command)
	}
	if snapshot.Screen.Focus != "prompt" {
		t.Fatalf("focus = %q, want prompt", snapshot.Screen.Focus)
	}
	regions := semanticRegionsByName(t, snapshot)
	command, ok := regions["command"]
	if !ok {
		t.Fatal("command region missing")
	}
	if !containsAll(strings.Join(command.Items, "\n"), []string{route, "command route: " + route, "route source: policy.command", commandFixtureMarker(route)}) {
		t.Fatalf("command region items = %v, want route, source, and marker content", command.Items)
	}
}

func commandFixtureMarker(route string) string {
	switch route {
	case "status":
		return "app-owned status inspection unavailable in presentation-only fallback"
	case "review":
		return "app-owned review inspection unavailable in presentation-only fallback"
	case "compact":
		return "app-owned manual compaction unavailable in presentation-only fallback"
	default:
		return "Deterministic placeholder"
	}
}

func TestPolicyRoutingFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		wantRender []string
		wantRegion []string
	}{
		{
			name:       "policy-explicit-route",
			wantRender: []string{"Policy routing:", "source: policy.capability.explicit_slash", "candidate: plan", "confidence: 100", "recommended successor: plan", "successor valid: true", "transition claimed: false", "requested effect: model_call operation=model.call target=primary model reason=runtime owns model loop"},
			wantRegion: []string{"source: policy.capability.explicit_slash", "candidate: plan", "confidence: 100", "recommended_successor: plan", "successor_valid: true", "transition_claimed: false", "boundary_request: artifact_access operation=artifact.access target=plan artifact reason=state resolver owns artifact access"},
		},
		{
			name:       "policy-natural-language-route",
			wantRender: []string{"Policy routing:", "source: policy.capability.natural_language", "input: please audit the boundary risks", "candidate: audit", "confidence: 86", "current phase: build", "successor valid: true"},
			wantRegion: []string{"candidate: audit", "confidence: 86", "reason: audit intent matched", "current_phase: build", "transition_claimed: false", "source_ref: policy-natural-language-route kind=prompt excerpt=please audit the boundary risks"},
		},
		{
			name:       "policy-waiting-route",
			wantRender: []string{"Policy routing:", "source: policy.capability.waiting", "candidate: brief", "confidence: 42", "runtime status: waiting", "needed input: Clarify whether you want a brief, discussion, plan, build, or audit.", "transition claimed: false"},
			wantRegion: []string{"candidate: brief", "confidence: 42", "runtime_status: waiting", "needed_input: Clarify whether you want a brief, discussion, plan, build, or audit.", "transition_claimed: false"},
		},
		{
			name:       "policy-invalid-successor",
			wantRender: []string{"Policy routing:", "source: policy.capability.successor_validation", "candidate: build", "recommended successor: deliberate", "successor rejected: true", "invalid workflow successor from", "transition claimed: false", "requested effect: permission_check operation=tool.write target=ROADMAP.md reason=permission owns approval"},
			wantRegion: []string{"candidate: build", "recommended_successor: deliberate", "successor_rejected: true", "successor_reason: invalid workflow successor from", "transition_claimed: false", "boundary_request: permission_check operation=tool.write target=ROADMAP.md reason=permission owns approval"},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := loadPolicyRoutingFixture(t, tc.name)
			assertFixtureSizes(t, fixture, policyRoutingFixtureSizes())
			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
					assertTextSnapshot(t, fixture, renderCase.file, got)
					plain := stripANSI(got)
					if !containsAll(plain, tc.wantRender) {
						t.Fatalf("%s policy routing render missing evidence %v:\n%s", tc.name, tc.wantRender, plain)
					}
					if containsAny(plain, []string{"provider call executed", "workflow phase changed", "permission approval: true", "artifact mutation: true"}) {
						t.Fatalf("%s policy routing render leaked execution or transition:\n%s", tc.name, plain)
					}
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
					if snapshot.PolicyRoute == nil || snapshot.PolicyRoute.TransitionClaimed || snapshot.PolicyRoute.Executed {
						t.Fatalf("semantic policy route = %+v, want visible non-executing non-transitioning", snapshot.PolicyRoute)
					}
					regions := semanticRegionsByName(t, snapshot)
					policyRoute, ok := regions["policy_route"]
					if !ok || !containsAll(strings.Join(policyRoute.Items, "\n"), tc.wantRegion) {
						t.Fatalf("%s semantic policy route missing %v in %+v", tc.name, tc.wantRegion, policyRoute)
					}
				})
			}
		})
	}
}

func TestPolicyRoutingSemanticSnapshotExposesCandidateWithoutTransition(t *testing.T) {
	t.Parallel()

	snapshot := Semantic(loadPolicyRoutingFixture(t, "policy-invalid-successor").State, Size{Width: 120, Height: 44})
	if snapshot.PolicyRoute == nil {
		t.Fatal("PolicyRoute semantic snapshot is nil")
	}
	if snapshot.PolicyRoute.Candidate != "build" || snapshot.PolicyRoute.Confidence != 100 || snapshot.PolicyRoute.RecommendedSuccessor != "deliberate" {
		t.Fatalf("policy route candidate = %+v", snapshot.PolicyRoute)
	}
	if !snapshot.PolicyRoute.SuccessorRejected || snapshot.PolicyRoute.SuccessorValid || snapshot.PolicyRoute.TransitionClaimed || snapshot.PolicyRoute.Executed {
		t.Fatalf("policy route successor/transition fields = %+v", snapshot.PolicyRoute)
	}
	if len(snapshot.PolicyRoute.SourceRefs) != 1 || snapshot.PolicyRoute.SourceRefs[0].Kind != "capability_exit" {
		t.Fatalf("policy route source refs = %+v", snapshot.PolicyRoute.SourceRefs)
	}
	if len(snapshot.PolicyRoute.BoundaryRequests) != 1 || snapshot.PolicyRoute.BoundaryRequests[0].Kind != "permission_check" {
		t.Fatalf("policy route boundary requests = %+v", snapshot.PolicyRoute.BoundaryRequests)
	}
}

func TestUtilityWorkerFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		status     string
		wantRender []string
		wantRegion []string
	}{
		{
			name:       "utility-idle",
			status:     "idle",
			wantRender: []string{"Utility worker:", "status: idle", "model: opencode-go/deepseek-v4-flash:max", "read-only: true", "summary: idle utility worker ready"},
			wantRegion: []string{"status: idle", "model: opencode-go/deepseek-v4-flash:max", "read_only: true", "file_mutation: false", "display-only"},
		},
		{
			name:       "utility-running",
			status:     "running",
			wantRender: []string{"Utility worker:", "status: running", "summary: fake utility job running", "file mutation: false", "workflow transition: false"},
			wantRegion: []string{"status: running", "summary: fake utility job running", "permission_approval: false", "final_judgment: false"},
		},
		{
			name:       "utility-result",
			status:     "completed",
			wantRender: []string{"Utility worker:", "status: completed", "summary: fake utility suggestion ready", "suggestion: Review current status before starting new background utility work. refs=utility-evidence-1", "utility evidence: utility-evidence-1 runtime_state app.status primary runtime idle; fake utility job only"},
			wantRegion: []string{"status: completed", "summary: fake utility suggestion ready", "suggestion: Review current status before starting new background utility work. refs=utility-evidence-1", "evidence: utility-evidence-1 runtime_state app.status primary runtime idle; fake utility job only"},
		},
		{
			name:       "utility-context-prep",
			status:     "completed",
			wantRender: []string{"Utility worker:", "job: context_prep status-context-prep", "summary: prepared context ready", "prepared context: Likely next context:", "prepared context non-authoritative: true", "utility evidence: context-prep-roadmap roadmap ROADMAP.md"},
			wantRegion: []string{"status: completed", "job: context_prep status-context-prep", "summary: prepared context ready", "prepared_context: Likely next context: roadmap M42 scope, current utility worker state, and recent status evidence. refs=context-prep-roadmap,context-prep-runtime", "prepared_context_non_authoritative: true"},
		},
		{
			name:       "utility-stale-context",
			status:     "completed",
			wantRender: []string{"Utility worker:", "job: stale_context_check status-stale-context-check", "summary: saved context appears stale", "stale context: stale", "stale context summary: saved context appears stale", "suggested next action: Rebuild foreground context before relying on saved context.", "context refresh: false"},
			wantRegion: []string{"status: completed", "job: stale_context_check status-stale-context-check", "stale_context_status: stale", "stale_context_summary: saved context appears stale refs=stale-context-saved,stale-context-current", "suggested_next_action: Rebuild foreground context before relying on saved context.", "context_refresh: false"},
		},
		{
			name:       "utility-summary-refresh",
			status:     "completed",
			wantRender: []string{"Utility worker:", "job: summary_refresh status-summary-refresh", "summary: summary refreshed with source-backed details", "summary refresh: refreshed", "refreshed summary: Runtime summary mentions status only.", "summary refresh source refs: summary-refresh-runtime, summary-refresh-roadmap", "summary refresh confidence: high", "summary refresh detail: primary runtime remains idle", "utility evidence: summary-refresh-source-1 source_ref app.status source_ref=summary-refresh-runtime"},
			wantRegion: []string{"status: completed", "job: summary_refresh status-summary-refresh", "summary_refresh_status: refreshed", "summary_refresh_refreshed: Runtime summary mentions status only. Important details: primary runtime remains idle; source refs stay visible refs=summary-refresh-runtime,summary-refresh-roadmap", "summary_refresh_confidence: high", "summary_refresh_detail: source refs stay visible", "final_judgment: false"},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := loadUtilityFixture(t, tc.name)
			assertFixtureSizes(t, fixture, utilityFixtureSizes())
			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
					assertTextSnapshot(t, fixture, renderCase.file, got)
					plain := stripANSI(got)
					if !containsAll(plain, tc.wantRender) {
						t.Fatalf("%s utility render missing evidence %v:\n%s", tc.name, tc.wantRender, plain)
					}
					if containsAny(plain, []string{"provider call", "workflow phase changed", "git write"}) {
						t.Fatalf("%s utility render leaked forbidden capability:\n%s", tc.name, plain)
					}
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
					if snapshot.Utility == nil || snapshot.Utility.Status != tc.status || snapshot.Utility.Model != "opencode-go/deepseek-v4-flash:max" || !snapshot.Utility.ReadOnly {
						t.Fatalf("utility semantic = %+v", snapshot.Utility)
					}
					if snapshot.Utility.Safety.FileMutation || snapshot.Utility.Safety.GitMutation || snapshot.Utility.Safety.ProjectArtifactMutation || snapshot.Utility.Safety.ApprovalGrant || snapshot.Utility.Safety.WorkflowPhaseTransition || snapshot.Utility.Safety.FinalJudgment || snapshot.Utility.Safety.ContextRefresh || snapshot.Utility.Safety.ContextCompaction || snapshot.Utility.Safety.ContextRewrite {
						t.Fatalf("utility semantic crossed safety boundary: %+v", snapshot.Utility.Safety)
					}
					regions := semanticRegionsByName(t, snapshot)
					utilityRegion := strings.Join(regions["utility"].Items, "\n")
					if !containsAll(utilityRegion, tc.wantRegion) {
						t.Fatalf("%s utility semantic region missing %v in %v", tc.name, tc.wantRegion, regions["utility"].Items)
					}
				})
			}
		})
	}
}

func TestCompactFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		status         string
		wantRender     []string
		wantSemantic   []string
		sourceRefCount int
	}{
		{
			name:   "compact-running",
			status: "running",
			wantRender: []string{
				"Compact:",
				"source: app.compact.background",
				"mode: background",
				"status: running",
				"summary: background context compaction running",
			},
			wantSemantic: []string{"mode: background", "status: running", "summary: background context compaction running", "app-owned", "display-only"},
		},
		{
			name:   "compact-result",
			status: "flagged",
			wantRender: []string{
				"Compact:",
				"status: flagged",
				"summary: manual compaction preserved 2 source refs",
				"caveat: input context warning: shell stdout truncated",
				"compact source ref: command-1-stdout-1 command_stdout command=git status --short excerpt=M internal[path-redacted]",
				"Context:",
				"status: compacted",
				"claim: manual compaction preserved 2 source refs",
			},
			wantSemantic:   []string{"mode: manual", "status: flagged", "summary: manual compaction preserved 2 source refs", "caveat: input context warning: shell stdout truncated", "source_ref: command-1-stdout-1 kind=command_stdout command=git status --short excerpt=M internal[path-redacted]"},
			sourceRefCount: 2,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := loadCompactFixture(t, tc.name)
			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
					assertTextSnapshot(t, fixture, renderCase.file, got)
					plain := stripANSI(got)
					if !containsAll(plain, tc.wantRender) {
						t.Fatalf("%s compact render missing evidence %v:\n%s", tc.name, tc.wantRender, plain)
					}
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
					if snapshot.Compact == nil || snapshot.Compact.Status != tc.status {
						t.Fatalf("compact semantic = %+v, want status %q", snapshot.Compact, tc.status)
					}
					if len(snapshot.Compact.SourceRefs) != tc.sourceRefCount {
						t.Fatalf("compact source refs = %+v, want %d", snapshot.Compact.SourceRefs, tc.sourceRefCount)
					}
					regions := semanticRegionsByName(t, snapshot)
					compactRegion := strings.Join(regions["compact"].Items, "\n")
					if !containsAll(compactRegion, tc.wantSemantic) {
						t.Fatalf("%s compact semantic region missing %v in %v", tc.name, tc.wantSemantic, regions["compact"].Items)
					}
					if tc.name == "compact-result" {
						contextRegion := strings.Join(regions["context"].Items, "\n")
						if snapshot.Context == nil || !containsAll(contextRegion, []string{"status: compacted", "claim: manual compaction preserved 2 source refs", "warning: input context warning: shell stdout truncated"}) {
							t.Fatalf("compact result context semantic = %+v region=%v", snapshot.Context, regions["context"].Items)
						}
					}
				})
			}
		})
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
				t.Fatalf("actions = %+v, want q quit only with no command-routing semantics", snapshot.Actions)
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
			if tc.wantRail && !containsAll(plain, []string{"Session", "phase source: " + testWorkflowPhaseSource, "primary model: placeholder"}) {
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
					if snapshot.Session.Phase != fixture.State.Phase || snapshot.Session.PhaseSource != fixture.State.PhaseSource {
						t.Fatalf("phase = %q from %q, want injected %q from %q", snapshot.Session.Phase, snapshot.Session.PhaseSource, fixture.State.Phase, fixture.State.PhaseSource)
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
	if snapshot.Session.Phase != testWorkflowPhaseLabel || snapshot.Session.PhaseSource != testWorkflowPhaseSource {
		t.Fatalf("phase = %q from %q, want injected %q from %q", snapshot.Session.Phase, snapshot.Session.PhaseSource, testWorkflowPhaseLabel, testWorkflowPhaseSource)
	}
	if snapshot.Session.Active || snapshot.Session.QueuedMessages != 0 {
		t.Fatalf("session implies runtime workflow behavior: %+v", snapshot.Session)
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
	if !containsAll(strings.Join(regions["phase"].Items, "\n"), []string{testWorkflowPhaseLabel, "display-only"}) {
		t.Fatalf("phase region items = %v, want injected display-only semantics", regions["phase"].Items)
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

func loadFetchToolFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	var state ViewState
	switch name {
	case "fetch-tool-running":
		state = fetchToolState(&FetchView{
			Name:     "fetch",
			Status:   "running",
			ReadOnly: true,
			URL:      "https://example.com/docs",
			Method:   "GET",
		})
	case "fetch-success":
		state = fetchToolState(&FetchView{
			Name:              "fetch",
			Status:            "completed",
			ReadOnly:          true,
			URL:               "https://example.com/docs",
			Method:            "GET",
			ExpectedEffect:    "read remote content through bounded fetch",
			HTTPStatusCode:    200,
			HTTPStatus:        "200 OK",
			ContentType:       "text/plain; charset=utf-8",
			PreviewLines:      []string{"# Aila docs", "Remote context preview with token=secret-value and /home/jgabor/.config/aila/config.toml"},
			PreviewTruncated:  true,
			OmittedBytesKnown: true,
			OmittedBytes:      42,
			TruncationMarker:  "preview_truncated",
			DurationMillis:    17,
		})
	case "fetch-failure":
		state = fetchToolState(&FetchView{
			Name:              "fetch",
			Status:            "http_error",
			ReadOnly:          true,
			URL:               "https://example.com/missing",
			Method:            "GET",
			ExpectedEffect:    "read remote content through bounded fetch",
			HTTPStatusCode:    404,
			HTTPStatus:        "404 Not Found",
			ContentType:       "text/plain",
			PreviewLines:      []string{"not found from /home/jgabor/git/aila/.aila/project.toml"},
			PreviewTruncated:  false,
			OmittedBytesKnown: true,
			OmittedBytes:      0,
			DurationMillis:    11,
			ErrorKind:         "http_status",
			ErrorMessage:      "remote returned 404 Not Found token=secret-value",
		})
	default:
		t.Fatalf("unknown fetch fixture %q", name)
	}
	state.Scenario = name
	return loadRenderFixture(t, name, state)
}

func TestM21FetchRenderAndSemantic(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		wantRender     []string
		forbiddenPlain []string
		completed      bool
	}{
		{
			name: "fetch-tool-running",
			wantRender: []string{
				"Fetch result:",
				"tool: fetch",
				"status: running",
				"read-only: true",
				"url: https://example.com/docs",
				"method: GET",
				"completed: false",
			},
			forbiddenPlain: []string{"remote status:", "preview:", "error kind:", "completed: true"},
		},
		{
			name: "fetch-success",
			wantRender: []string{
				"Fetch result:",
				"status: completed",
				"read-only: true",
				"url: https://example.com/docs",
				"expected effect: read remote content through bounded fetch",
				"remote status: 200",
				"content type: text/plain; charset=utf-8",
				"# Aila docs",
				"[redacted]",
				"preview truncated: true",
				"omitted bytes: 42",
				"truncation marker: preview_truncated",
			},
			completed: true,
		},
		{
			name: "fetch-failure",
			wantRender: []string{
				"Fetch result:",
				"status: http_error",
				"read-only: true",
				"url: https://example.com/missing",
				"remote status: 404",
				"remote status text: 404 Not Found",
				"[path-redacted]",
				"error kind: http_status",
				"error message: remote returned 404 Not Found [redacted]",
			},
			completed: true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := loadFetchToolFixture(t, tc.name).State
			render := RenderPlain(state, Size{Width: 120, Height: 44})
			if !containsAll(render, tc.wantRender) {
				t.Fatalf("%s render missing fetch evidence %v:\n%s", tc.name, tc.wantRender, render)
			}
			if containsAny(render, tc.forbiddenPlain) {
				t.Fatalf("%s render implies wrong fetch state:\n%s", tc.name, render)
			}
			assertNoReadLeak(t, render)

			snapshot := Semantic(state, Size{Width: 120, Height: 44})
			if snapshot.Fetch == nil || !snapshot.Fetch.ReadOnly || snapshot.Fetch.Completed != tc.completed {
				t.Fatalf("fetch semantic = %+v, want read-only completed=%v", snapshot.Fetch, tc.completed)
			}
			regions := semanticRegionsByName(t, snapshot)
			fetchRegion := strings.Join(regions["fetch_tool"].Items, "\n")
			if !containsAll(fetchRegion, []string{"read_only: true", "app-owned", "display-only"}) {
				t.Fatalf("fetch semantic region = %v", regions["fetch_tool"].Items)
			}
			assertNoReadLeak(t, RenderSemanticJSON(state, Size{Width: 120, Height: 44}))
		})
	}
}

func TestM21FetchFixtureSnapshots(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"fetch-tool-running", "fetch-success", "fetch-failure"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := loadFetchToolFixture(t, name)
			if fixture.Kind != "static_shell" || fixture.TerminalBehavior != "bubbletea_static" || fixture.QuitInput != "q" {
				t.Fatalf("fetch fixture metadata = %+v", fixture)
			}
			for _, renderCase := range fixture.TextCases() {
				renderCase := renderCase
				t.Run(renderCase.name, func(t *testing.T) {
					t.Parallel()

					got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
					assertTextSnapshot(t, fixture, renderCase.file, got)
					plain := stripANSI(got)
					if !containsAll(plain, []string{"Fetch result:", "read-only: true"}) {
						t.Fatalf("%s fixture render missing fetch evidence:\n%s", name, plain)
					}
					assertNoReadLeak(t, plain)
				})
			}

			for _, semanticCase := range fixture.SemanticCases() {
				semanticCase := semanticCase
				t.Run(semanticCase.name, func(t *testing.T) {
					t.Parallel()

					got := RenderSemanticJSON(fixture.State, semanticCase.size)
					assertSemanticSnapshot(t, fixture, semanticCase.file, got)
					assertNoReadLeak(t, got)
					var snapshot SemanticSnapshot
					if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
						t.Fatalf("unmarshal semantic snapshot: %v", err)
					}
					if snapshot.Fetch == nil || !snapshot.Fetch.ReadOnly {
						t.Fatalf("semantic fetch = %+v", snapshot.Fetch)
					}
				})
			}
		})
	}
}

func TestM21FetchPTYSmokeDecision(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"/fetch https://example.com", "fetch https://example.com", "curl https://example.com"} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			model := NewModelWithStateSizePromptSubmitAndCommandRoute(IdleEmptyState(), Size{Width: 80, Height: 24}, nil, nil)
			for _, r := range input {
				updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				if cmd != nil {
					t.Fatalf("typing %q emitted command", input)
				}
				model = updated.(Model)
			}
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatalf("submitting %q emitted command", input)
			}
			state := updated.(Model).state
			if state.Fetch != nil || state.Read != nil || state.Search != nil || state.Command != nil || state.CommandRoute != "" || state.SurfaceTitle != "" || state.RuntimeStatus != "" {
				t.Fatalf("%q unexpectedly invoked visible fetch state: %+v", input, state)
			}
		})
	}
}

func fetchToolState(fetch *FetchView) ViewState {
	state := IdleEmptyState()
	state.Phase = testWorkflowPhaseLabel
	state.PhaseSource = testWorkflowPhaseSource
	state.RuntimeStatus = "idle"
	if fetch != nil && fetch.Status == "running" {
		state.RuntimeStatus = "active"
		state.RuntimeActive = true
	}
	state.StatusSource = "runtime.dispatch"
	state.StatusDetail = "fetch tool dispatch"
	state.Autonomy = "read"
	state.Fetch = fetch
	return state
}

func blockedReadDecisionView() *DecisionView {
	return &DecisionView{
		Autonomy:         "off",
		Source:           "autonomy_policy",
		Allowed:          false,
		Automatic:        false,
		ApprovalRequired: true,
		Reason:           "autonomy off requires approval",
		OperationKind:    "read",
		Name:             "read",
		Target:           "internal/tui/render.go",
		ExpectedEffect:   "bounded workspace file preview",
		Reversible:       true,
	}
}

func loadBlockedReadDecisionFixture(t *testing.T) renderFixture {
	t.Helper()

	state := readToolState(&ReadView{
		Name:         "read",
		Status:       "failed",
		ReadOnly:     true,
		Path:         "internal/tui/render.go",
		ErrorKind:    "permission_denied",
		ErrorMessage: "autonomy off requires approval",
		Decision:     blockedReadDecisionView(),
	})
	state.Scenario = "blocked-read-decision"
	state.Autonomy = "off"
	return loadRenderFixture(t, state.Scenario, state)
}

func TestM22BlockedReadDecisionRenderAndSemantic(t *testing.T) {
	t.Parallel()

	fixture := loadBlockedReadDecisionFixture(t)
	render := RenderPlain(fixture.State, Size{Width: 120, Height: 44})
	wantRender := []string{
		"Read tool:",
		"status: failed",
		"read-only: true",
		"path: internal/tui/render.go",
		"error kind: permission_denied",
		"decision source: autonomy_policy",
		"decision: denied",
		"decision automatic: false",
		"approval required: true",
		"autonomy: off",
		"operation: read",
		"decision tool: read",
		"decision target: internal/tui/render.go",
		"decision expected effect: bounded workspace file preview",
		"decision reversible: true",
		"decision reason: autonomy off requires approval",
	}
	if !containsAll(render, wantRender) {
		t.Fatalf("blocked decision render missing evidence %v:\n%s", wantRender, render)
	}
	if containsAny(render, []string{"approval prompt", "approve action", "write class"}) {
		t.Fatalf("blocked decision render implies out-of-scope approval behavior:\n%s", render)
	}
	assertNoReadLeak(t, render)

	snapshot := Semantic(fixture.State, Size{Width: 120, Height: 44})
	if snapshot.Session.Autonomy != "off" || snapshot.Read == nil || snapshot.Read.Decision == nil {
		t.Fatalf("semantic blocked decision missing autonomy/read decision: %+v", snapshot)
	}
	decision := snapshot.Read.Decision
	if decision.Source != "autonomy_policy" || decision.Allowed || decision.Automatic || !decision.ApprovalRequired || decision.OperationKind != "read" || decision.Name != "read" || decision.Target != "internal/tui/render.go" || decision.ExpectedEffect == "" || !decision.Reversible || decision.Reason != "autonomy off requires approval" {
		t.Fatalf("semantic decision = %+v", decision)
	}
	regions := semanticRegionsByName(t, snapshot)
	readRegion := strings.Join(regions["read_tool"].Items, "\n")
	if !containsAll(readRegion, []string{"decision_source: autonomy_policy", "decision: denied", "approval_required: true", "operation_kind: read", "decision_reason: autonomy off requires approval"}) {
		t.Fatalf("read semantic region missing decision evidence: %v", regions["read_tool"].Items)
	}
	assertNoReadLeak(t, RenderSemanticJSON(fixture.State, Size{Width: 120, Height: 44}))
}

func TestM22BlockedReadDecisionFixtureSnapshots(t *testing.T) {
	t.Parallel()

	fixture := loadBlockedReadDecisionFixture(t)
	if fixture.Kind != "static_shell" || fixture.TerminalBehavior != "bubbletea_static" || fixture.QuitInput != "q" {
		t.Fatalf("blocked decision fixture metadata = %+v", fixture)
	}
	for _, renderCase := range fixture.TextCases() {
		renderCase := renderCase
		t.Run(renderCase.name, func(t *testing.T) {
			t.Parallel()

			got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
			assertTextSnapshot(t, fixture, renderCase.file, got)
			plain := stripANSI(got)
			if !containsAll(plain, []string{"Read tool:", "decision source: autonomy_policy", "decision: denied", "approval required: true"}) {
				t.Fatalf("blocked decision fixture render missing evidence:\n%s", plain)
			}
			assertNoReadLeak(t, plain)
		})
	}

	for _, semanticCase := range fixture.SemanticCases() {
		semanticCase := semanticCase
		t.Run(semanticCase.name, func(t *testing.T) {
			t.Parallel()

			got := RenderSemanticJSON(fixture.State, semanticCase.size)
			assertSemanticSnapshot(t, fixture, semanticCase.file, got)
			assertNoReadLeak(t, got)
			var snapshot SemanticSnapshot
			if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
				t.Fatalf("unmarshal semantic snapshot: %v", err)
			}
			if snapshot.Session.Autonomy != "off" || snapshot.Read == nil || snapshot.Read.Decision == nil || snapshot.Read.Decision.Source != "autonomy_policy" {
				t.Fatalf("semantic blocked decision = %+v", snapshot.Read)
			}
		})
	}
}

func TestM22DecisionPTYSmokeDecision(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"/autonomy off", "autonomy read", "approve read"} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			model := NewModelWithStateSizePromptSubmitAndCommandRoute(IdleEmptyState(), Size{Width: 80, Height: 24}, nil, nil)
			for _, r := range input {
				updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				if cmd != nil {
					t.Fatalf("typing %q emitted command", input)
				}
				model = updated.(Model)
			}
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatalf("submitting %q emitted command", input)
			}
			state := updated.(Model).state
			if state.Read != nil || state.Command != nil || state.Search != nil || state.Fetch != nil || state.RuntimeStatus != "" {
				t.Fatalf("%q unexpectedly invoked visible decision state: %+v", input, state)
			}
		})
	}
}

func loadStreamingAssistantFixture(t *testing.T) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Scenario = "streaming-assistant"
	state.Phase = "BUILD"
	state.PhaseSource = "workflow.fixture"
	state.RuntimeStatus = "active"
	state.StatusSource = "runtime.fixture"
	state.StatusDetail = "agent event adapter"
	state.RuntimeActive = true
	state.Transcript = []TranscriptTurn{{
		UserText:           "explain streaming",
		AssistantText:      "Streaming partial answer token=secret-value from /home/jgabor/.config/aila/config.toml",
		AssistantStreaming: true,
		AssistantSource:    "fake-provider",
		AssistantModel:     "fake-model",
	}}
	return loadRenderFixture(t, state.Scenario, state)
}

func TestM23StreamingAssistantRenderAndSemantic(t *testing.T) {
	t.Parallel()

	fixture := loadStreamingAssistantFixture(t)
	render := RenderPlain(fixture.State, Size{Width: 120, Height: 44})
	wantRender := []string{
		"Runtime active",
		"assistant streaming: Streaming partial answer [redacted] from [path-redacted]",
		"assistant status: incomplete",
		"assistant source: fake-provider fake-model",
	}
	if !containsAll(render, wantRender) {
		t.Fatalf("streaming render missing evidence %v:\n%s", wantRender, render)
	}
	if containsAny(render, []string{"token=secret-value", "/home/jgabor", "completed answer"}) {
		t.Fatalf("streaming render leaked or claimed completion:\n%s", render)
	}

	snapshot := Semantic(fixture.State, Size{Width: 120, Height: 44})
	if !snapshot.Session.Active || snapshot.Session.RuntimeStatus != "active" {
		t.Fatalf("streaming session = %+v", snapshot.Session)
	}
	regions := semanticRegionsByName(t, snapshot)
	chat := strings.Join(regions["chat"].Items, "\n")
	if !containsAll(chat, []string{"assistant_streaming: true", "assistant_incomplete: true", "assistant: Streaming partial answer [redacted] from [path-redacted]", "assistant_source: fake-provider", "assistant_model: fake-model"}) {
		t.Fatalf("streaming semantic chat = %v", regions["chat"].Items)
	}
	assertNoReadLeak(t, RenderSemanticJSON(fixture.State, Size{Width: 120, Height: 44}))
}

func TestM23StreamingAssistantFixtureSnapshots(t *testing.T) {
	t.Parallel()

	fixture := loadStreamingAssistantFixture(t)
	if fixture.Kind != "static_shell" || fixture.TerminalBehavior != "bubbletea_static" || fixture.QuitInput != "q" {
		t.Fatalf("streaming fixture metadata = %+v", fixture)
	}
	for _, renderCase := range fixture.TextCases() {
		renderCase := renderCase
		t.Run(renderCase.name, func(t *testing.T) {
			t.Parallel()

			got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
			assertTextSnapshot(t, fixture, renderCase.file, got)
			plain := stripANSI(got)
			if !containsAll(plain, []string{"assistant streaming:", "assistant status: incomplete"}) {
				t.Fatalf("streaming fixture render missing evidence:\n%s", plain)
			}
			assertNoReadLeak(t, plain)
		})
	}
	for _, semanticCase := range fixture.SemanticCases() {
		semanticCase := semanticCase
		t.Run(semanticCase.name, func(t *testing.T) {
			t.Parallel()

			got := RenderSemanticJSON(fixture.State, semanticCase.size)
			assertSemanticSnapshot(t, fixture, semanticCase.file, got)
			assertNoReadLeak(t, got)
		})
	}
}

func buildActiveFixtureSizes() []fixtureSize {
	return []fixtureSize{{Name: "80x24", Width: 80, Height: 24}, {Name: "120x32", Width: 120, Height: 32}, {Name: "160x45", Width: 160, Height: 45}}
}

func readOnlyAgentFixtureSizes() []fixtureSize {
	return []fixtureSize{{Name: "80x24", Width: 80, Height: 24}, {Name: "120x32", Width: 120, Height: 32}}
}

func loadReadOnlyBuildActiveFixture(t *testing.T) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Scenario = "build-active"
	state.Phase = "BUILD"
	state.PhaseSource = "workflow.fixture"
	state.PrimaryModel = "fake/fake-readonly"
	state.UtilityModel = "placeholder"
	state.Autonomy = "read"
	state.RuntimeStatus = "active"
	state.RuntimeActive = true
	state.SurfaceTitle = "agent evidence"
	state.Read = &ReadView{Name: "read", Status: "running", ReadOnly: true, Path: "README.md", RequestedRange: ReadLineRangeView{Limit: 6}}
	state.Transcript = []TranscriptTurn{{UserText: "summarize the build", AssistantText: "I will inspect README.md before answering.", AssistantStreaming: true}}
	return loadRenderFixture(t, state.Scenario, state)
}

func loadReadOnlyProviderFailureFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	code := "provider_auth_failed"
	message := "provider authentication failed"
	retryable := false
	switch name {
	case "provider-auth-failed":
		code = "provider_auth_failed"
	case "provider-timeout":
		code = "provider_timeout"
		message = "provider request timed out"
		retryable = true
	case "rate-limited":
		code = "rate_limited"
		message = "provider rate limit reached"
		retryable = true
	case "model-unavailable":
		code = "model_unavailable"
		message = "model unavailable"
	default:
		t.Fatalf("unknown read-only provider failure fixture %q", name)
	}
	state := IdleEmptyState()
	state.Scenario = name
	state.Phase = "BUILD"
	state.PhaseSource = "workflow.fixture"
	state.PrimaryModel = "fake/fake-readonly"
	state.Autonomy = "read"
	state.RuntimeStatus = "idle"
	state.RuntimeResult = message
	state.SurfaceTitle = "agent evidence"
	state.Transcript = []TranscriptTurn{{UserText: "summarize the build", AssistantText: message, AssistantSource: "fake", AssistantModel: "fake-readonly"}}
	state.Diagnostics = []DiagnosticView{{Severity: "error", Source: "provider", RecoveryAction: "check provider configuration", AffectedArtifact: "provider_request", UserInputNeeded: !retryable, BoundedMessage: code + ": " + message + " retryable=" + boolLabel(retryable)}}
	return loadRenderFixture(t, state.Scenario, state)
}

func TestReadOnlyBuildActiveFixtureSnapshots(t *testing.T) {
	fixture := loadReadOnlyBuildActiveFixture(t)
	assertFixtureSizes(t, fixture, buildActiveFixtureSizes())
	for _, renderCase := range fixture.TextCases() {
		got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
		assertTextSnapshot(t, fixture, renderCase.file, got)
		plain := stripANSI(got)
		if !containsAll(plain, []string{"Runtime active", "Model fake/fake-readonly", "Read tool:", "status: running", "assistant streaming:", "assistant status: incomplete"}) {
			t.Fatalf("read-only build-active render missing evidence:\n%s", plain)
		}
	}
	for _, semanticCase := range fixture.SemanticCases() {
		got := RenderSemanticJSON(fixture.State, semanticCase.size)
		assertSemanticSnapshot(t, fixture, semanticCase.file, got)
		var snapshot SemanticSnapshot
		if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
			t.Fatalf("unmarshal semantic snapshot: %v", err)
		}
		if !snapshot.Session.Active || snapshot.Read == nil || snapshot.Read.Status != "running" || snapshot.Session.PrimaryModel != "fake/fake-readonly" {
			t.Fatalf("build-active semantic = %+v", snapshot)
		}
	}
}

func TestReadOnlyProviderFailureFixtureSnapshots(t *testing.T) {
	for _, name := range []string{"provider-auth-failed", "provider-timeout", "rate-limited", "model-unavailable"} {
		name := name
		t.Run(name, func(t *testing.T) {
			fixture := loadReadOnlyProviderFailureFixture(t, name)
			assertFixtureSizes(t, fixture, readOnlyAgentFixtureSizes())
			for _, renderCase := range fixture.TextCases() {
				got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
				assertTextSnapshot(t, fixture, renderCase.file, got)
				plain := stripANSI(got)
				if !containsAll(plain, []string{"Diagnostics:", "source: provider", "affected artifact: provider_request", "assistant source: fake fake-readonly"}) {
					t.Fatalf("read-only provider failure render missing evidence:\n%s", plain)
				}
			}
			for _, semanticCase := range fixture.SemanticCases() {
				got := RenderSemanticJSON(fixture.State, semanticCase.size)
				assertSemanticSnapshot(t, fixture, semanticCase.file, got)
				var snapshot SemanticSnapshot
				if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
					t.Fatalf("unmarshal semantic snapshot: %v", err)
				}
				if len(snapshot.Diagnostics) != 1 || snapshot.Diagnostics[0].Source != "provider" || snapshot.Diagnostics[0].AffectedArtifact != "provider_request" {
					t.Fatalf("provider failure semantic = %+v", snapshot.Diagnostics)
				}
			}
		})
	}
}

func TestM23StreamingPTYSmokeDecision(t *testing.T) {
	t.Parallel()

	model := NewModelWithStateSizePromptSubmitAndCommandRoute(IdleEmptyState(), Size{Width: 80, Height: 24}, nil, nil)
	for _, r := range "/stream fake" {
		updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if cmd != nil {
			t.Fatalf("typing emitted command")
		}
		model = updated.(Model)
	}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("submitting emitted command")
	}
	if state := updated.(Model).state; state.RuntimeStatus != "" || len(state.Transcript) != 0 {
		t.Fatalf("streaming input unexpectedly invoked visible state: %+v", state)
	}
}

func approvalPendingFixtureSizes() []fixtureSize {
	return []fixtureSize{{Name: "80x24", Width: 80, Height: 24}, {Name: "80x44", Width: 80, Height: 44}, {Name: "120x32", Width: 120, Height: 32}, {Name: "120x44", Width: 120, Height: 44}, {Name: "160x45", Width: 160, Height: 45}}
}

func loadApprovalPendingFixture(t *testing.T) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Scenario = "approval-pending"
	state.Phase = "BUILD"
	state.PhaseSource = "workflow.fixture"
	state.PrimaryModel = "fake/fake-readonly"
	state.Autonomy = "read"
	state.RuntimeStatus = "approval-pending"
	state.StatusSource = "runtime.fixture"
	state.StatusDetail = "approval pending"
	state.RuntimeActive = true
	state.RuntimeResult = "approval pending: internal/demo.txt"
	state.Approval = &ApprovalProposalView{
		ID:             "fake-approval-001",
		OperationKind:  "file_mutation",
		Target:         "internal/demo.txt",
		RiskSummary:    "Would update a workspace file if mutation execution existed.",
		PreviewLines:   []string{"write requested by fake proposal", "default is deny until write classes exist"},
		DefaultAction:  "deny",
		Path:           "internal/demo.txt",
		Command:        []string{"write", "internal/demo.txt"},
		WorkingDir:     ".",
		ExpectedEffect: "preview only; no mutation execution in approval fixture",
		DiffPreview:    []string{"--- internal/demo.txt", "+++ internal/demo.txt", "@@", "-old", "+new"},
		Reversible:     true,
		RunID:          "run-fake-approval",
		Capability:     "approval-fixture",
	}
	return loadRenderFixture(t, state.Scenario, state)
}

func TestApprovalPendingFixtureSnapshots(t *testing.T) {
	fixture := loadApprovalPendingFixture(t)
	assertFixtureSizes(t, fixture, approvalPendingFixtureSizes())
	for _, renderCase := range fixture.TextCases() {
		got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
		assertTextSnapshot(t, fixture, renderCase.file, got)
		plain := stripANSI(got)
		required := []string{"Approval pending:", "proposal id: fake-approval-001", "operation kind: file_mutation"}
		if renderCase.size.Height >= 32 {
			required = append(required, "target: internal/demo.txt", "path: internal/demo.txt", "command: write internal/demo.txt", "default action: deny")
		}
		if renderCase.size.Height >= 44 {
			required = append(required, "diff preview:", "-old", "+new", "choices: a approve | n deny | d defer", "mutation executed: false")
		}
		if !containsAll(plain, required) {
			t.Fatalf("approval fixture render missing evidence:\n%s", plain)
		}
	}
	for _, semanticCase := range fixture.SemanticCases() {
		got := RenderSemanticJSON(fixture.State, semanticCase.size)
		assertSemanticSnapshot(t, fixture, semanticCase.file, got)
		var snapshot SemanticSnapshot
		if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
			t.Fatalf("unmarshal semantic snapshot: %v", err)
		}
		if snapshot.Approval == nil || snapshot.Approval.Path != "internal/demo.txt" || strings.Join(snapshot.Approval.Command, " ") != "write internal/demo.txt" || snapshot.Approval.DefaultAction != "deny" || snapshot.Approval.MutationExecuted {
			t.Fatalf("approval semantic = %+v", snapshot.Approval)
		}
		regions := semanticRegionsByName(t, snapshot)
		approval := strings.Join(regions["approval"].Items, "\n")
		if !containsAll(approval, []string{"proposal_id: fake-approval-001", "diff_preview_line: -old", "diff_preview_line: +new", "choice: approve input=a", "choice: deny input=n", "choice: defer input=d", "display-only"}) {
			t.Fatalf("approval semantic region = %v", regions["approval"].Items)
		}
	}
}

func TestApprovalKeysEmitDecisionMessagesOnly(t *testing.T) {
	for _, tc := range []struct {
		key    string
		action string
	}{
		{key: "a", action: "approve"},
		{key: "n", action: "deny"},
		{key: "d", action: "defer"},
	} {
		tc := tc
		t.Run(tc.action, func(t *testing.T) {
			state := IdleEmptyState()
			state.Approval = &ApprovalProposalView{ID: "approval-1", Target: "internal/demo.txt", DefaultAction: "deny"}
			var decisions []ApprovalDecisionInput
			model := NewModelWithStateSizePromptSubmitCommandRouteInterruptAndApproval(state, Size{Width: 80, Height: 44}, nil, nil, nil, func(decision ApprovalDecisionInput) TranscriptTurn {
				decisions = append(decisions, decision)
				return TranscriptTurn{RuntimeStatus: "idle", RuntimeResult: "approval " + decision.Action + ": internal/demo.txt", ApprovalDecision: &ApprovalDecisionView{ProposalID: decision.ProposalID, Action: decision.Action}}
			})

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)})
			if cmd != nil {
				t.Fatal("approval key emitted Bubble Tea command")
			}
			got := updated.(Model)
			if len(decisions) != 1 || decisions[0].ProposalID != "approval-1" || decisions[0].Action != tc.action {
				t.Fatalf("approval decisions = %+v", decisions)
			}
			if got.state.RuntimeResult != "approval "+tc.action+": internal/demo.txt" || got.state.Approval != nil {
				t.Fatalf("approval key state = %+v", got.state)
			}
		})
	}
}

func writePermissionFixtureSizes() []fixtureSize {
	return []fixtureSize{{Name: "80x44", Width: 80, Height: 44}, {Name: "120x44", Width: 120, Height: 44}}
}

func loadWritePermissionDecisionFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	var autonomy string
	var reason string
	var runID string
	switch name {
	case "write-permission-decision":
		autonomy = "write"
		reason = "write autonomy allows classified operation"
		runID = "run-write-permission"
	case "yolo-permission-decision":
		autonomy = "yolo"
		reason = "yolo autonomy grants classified operation"
		runID = "run-yolo-permission"
	default:
		t.Fatalf("unknown write permission fixture %q", name)
	}

	state := commandToolState(&CommandView{
		Name:           "bash",
		Status:         "proposed",
		ReadOnly:       false,
		Argv:           []string{"sh", "-c", "printf updated > internal/demo.txt"},
		WorkingDir:     ".",
		CommandFamily:  "shell mutation",
		ExpectedEffect: "would update internal/demo.txt; not executed by permission decision fixture",
		Decision: &DecisionView{
			Autonomy:         autonomy,
			Source:           "autonomy_policy",
			Allowed:          true,
			Automatic:        true,
			ApprovalRequired: false,
			Reason:           reason,
			OperationKind:    "exec",
			Name:             "bash",
			Command:          []string{"sh", "-c", "printf updated > internal/demo.txt"},
			WorkingDir:       ".",
			ExpectedEffect:   "would update internal/demo.txt; not executed by permission decision fixture",
			Reversible:       false,
			RunID:            runID,
			Capability:       "write-permission-fixture",
		},
	})
	state.Scenario = name
	state.Autonomy = autonomy
	state.RuntimeStatus = "permission-evaluated"
	state.StatusDetail = "write-shaped proposal classified only"
	return loadRenderFixture(t, name, state)
}

func TestWritePermissionDecisionRenderAndSemantic(t *testing.T) {
	for _, tc := range []struct {
		name     string
		autonomy string
		reason   string
	}{
		{name: "write-permission-decision", autonomy: "write", reason: "write autonomy allows classified operation"},
		{name: "yolo-permission-decision", autonomy: "yolo", reason: "yolo autonomy grants classified operation"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fixture := loadWritePermissionDecisionFixture(t, tc.name)
			render := RenderPlain(fixture.State, Size{Width: 120, Height: 44})
			if !containsAll(render, []string{
				"Bash command:",
				"status: proposed",
				"read-only: false",
				"command: sh -c printf updated > internal/demo.txt",
				"completed: false",
				"expected effect: would update internal/demo.txt; not executed by permission decision fixture",
				"decision source: autonomy_policy",
				"decision: allowed",
				"decision automatic: true",
				"approval required: false",
				"autonomy: " + tc.autonomy,
				"operation: exec",
				"decision tool: bash",
				"decision reason: " + tc.reason,
			}) {
				t.Fatalf("write permission render missing evidence:\n%s", render)
			}
			if containsAny(render, []string{"exit code:", "stdout:", "stderr:", "mutation executed: true"}) {
				t.Fatalf("write permission render implies execution:\n%s", render)
			}

			snapshot := Semantic(fixture.State, Size{Width: 120, Height: 44})
			if snapshot.Session.Autonomy != tc.autonomy || snapshot.Bash == nil || snapshot.Bash.Decision == nil {
				t.Fatalf("semantic write permission missing bash decision: %+v", snapshot)
			}
			if snapshot.Bash.Completed || snapshot.Bash.Decision.Source != "autonomy_policy" || !snapshot.Bash.Decision.Allowed || !snapshot.Bash.Decision.Automatic || snapshot.Bash.Decision.ApprovalRequired || snapshot.Bash.Decision.Autonomy != tc.autonomy || snapshot.Bash.Decision.OperationKind != "exec" || snapshot.Bash.Decision.Reason != tc.reason {
				t.Fatalf("semantic write decision = %+v bash=%+v", snapshot.Bash.Decision, snapshot.Bash)
			}
			regions := semanticRegionsByName(t, snapshot)
			bashRegion := strings.Join(regions["bash_tool"].Items, "\n")
			if !containsAll(bashRegion, []string{"status: proposed", "completed: false", "decision_source: autonomy_policy", "decision: allowed", "approval_required: false", "autonomy: " + tc.autonomy, "operation_kind: exec", "display-only"}) {
				t.Fatalf("write permission semantic region missing evidence: %v", regions["bash_tool"].Items)
			}
		})
	}
}

func TestWritePermissionFixtureSnapshots(t *testing.T) {
	for _, name := range []string{"write-permission-decision", "yolo-permission-decision"} {
		name := name
		t.Run(name, func(t *testing.T) {
			fixture := loadWritePermissionDecisionFixture(t, name)
			assertFixtureSizes(t, fixture, writePermissionFixtureSizes())
			for _, renderCase := range fixture.TextCases() {
				got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
				assertTextSnapshot(t, fixture, renderCase.file, got)
				plain := stripANSI(got)
				if !containsAll(plain, []string{"Bash command:", "status: proposed", "completed: false", "decision source: autonomy_policy", "approval required: false"}) {
					t.Fatalf("%s fixture render missing policy evidence:\n%s", name, plain)
				}
				if containsAny(plain, []string{"exit code:", "mutation executed: true"}) {
					t.Fatalf("%s fixture render implies execution:\n%s", name, plain)
				}
			}
			for _, semanticCase := range fixture.SemanticCases() {
				got := RenderSemanticJSON(fixture.State, semanticCase.size)
				assertSemanticSnapshot(t, fixture, semanticCase.file, got)
				var snapshot SemanticSnapshot
				if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
					t.Fatalf("unmarshal semantic snapshot: %v", err)
				}
				if snapshot.Bash == nil || snapshot.Bash.Decision == nil || snapshot.Bash.Completed || snapshot.Bash.Decision.ApprovalRequired {
					t.Fatalf("semantic write permission = %+v", snapshot.Bash)
				}
			}
		})
	}
}

func TestWritePermissionInputsDoNotSwitchAutonomyOrExecute(t *testing.T) {
	for _, input := range []string{"/autonomy write", "autonomy yolo", "approve write"} {
		input := input
		t.Run(input, func(t *testing.T) {
			model := NewModelWithStateSizePromptSubmitAndCommandRoute(IdleEmptyState(), Size{Width: 80, Height: 24}, nil, nil)
			for _, r := range input {
				updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				if cmd != nil {
					t.Fatalf("typing %q emitted command", input)
				}
				model = updated.(Model)
			}
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatalf("submitting %q emitted command", input)
			}
			state := updated.(Model).state
			if state.Command != nil || state.Approval != nil || state.ApprovalDecision != nil || state.CommandRoute != "" || state.RuntimeStatus != "" || len(state.Transcript) != 0 {
				t.Fatalf("%q unexpectedly changed visible autonomy or mutation state: %+v", input, state)
			}
		})
	}
}

func recoveryResultFixtureSizes() []fixtureSize {
	return []fixtureSize{{Name: "80x24", Width: 80, Height: 24}, {Name: "120x32", Width: 120, Height: 32}}
}

func loadRecoveryResultFixture(t *testing.T) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = "BUILD"
	state.PhaseSource = "workflow.fixture"
	state.Scenario = "recovery-result"
	state.RuntimeStatus = "idle"
	state.StatusSource = "runtime.dispatch"
	state.StatusDetail = "recovery command dispatch"
	state.Autonomy = "write"
	state = ApplyRecoveryView(state, &RecoveryView{
		Command:         "undo",
		Status:          "completed",
		TargetEventID:   "evt-fake-006",
		Action:          "delete_created_file",
		Paths:           []string{"notes.txt"},
		PreviousVersion: "sha256:fixture-new-version",
		NewVersion:      "missing",
		RedoAvailable:   true,
		RedoAction:      "restore_created_file",
		Decision: &DecisionView{
			Autonomy:         "write",
			Source:           "autonomy_policy",
			Allowed:          true,
			Automatic:        true,
			ApprovalRequired: false,
			Reason:           "write autonomy allows classified operation",
			OperationKind:    "mutation",
			Name:             "undo",
			Target:           "notes.txt",
			ExpectedEffect:   "delete created file for undo",
			Reversible:       true,
			RunID:            "current",
			Capability:       "recovery.undo",
		},
	})
	return loadRenderFixture(t, state.Scenario, state)
}

func TestRecoveryResultRenderAndSemantic(t *testing.T) {
	fixture := loadRecoveryResultFixture(t)
	render := RenderPlain(fixture.State, Size{Width: 120, Height: 32})
	if !containsAll(render, []string{"Recovery result:", "command: undo", "status: completed", "target event id: evt-fake-006", "action: delete_created_file", "paths: notes.txt", "redo available: true", "redo action: restore_created_file", "decision source: autonomy_policy"}) {
		t.Fatalf("recovery render missing evidence:\n%s", render)
	}
	snapshot := Semantic(fixture.State, Size{Width: 120, Height: 32})
	if snapshot.Recovery == nil || snapshot.Recovery.Command != "undo" || snapshot.Recovery.Status != "completed" || snapshot.Recovery.TargetEventID != "evt-fake-006" || !snapshot.Recovery.RedoAvailable || snapshot.Recovery.Decision == nil || snapshot.Recovery.Decision.Source != "autonomy_policy" {
		t.Fatalf("recovery semantic = %+v", snapshot.Recovery)
	}
	regions := semanticRegionsByName(t, snapshot)
	recoveryRegion := strings.Join(regions["recovery"].Items, "\n")
	if !containsAll(recoveryRegion, []string{"command: undo", "status: completed", "target_event_id: evt-fake-006", "paths: notes.txt", "redo_available: true", "redo_action: restore_created_file", "decision_source: autonomy_policy", "app-owned", "display-only"}) {
		t.Fatalf("recovery semantic region = %v", regions["recovery"].Items)
	}
}

func TestRecoveryResultFixtureSnapshots(t *testing.T) {
	fixture := loadRecoveryResultFixture(t)
	assertFixtureSizes(t, fixture, recoveryResultFixtureSizes())
	for _, renderCase := range fixture.TextCases() {
		got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
		assertTextSnapshot(t, fixture, renderCase.file, got)
		plain := stripANSI(got)
		if !containsAll(plain, []string{"Recovery result:", "command: undo", "status: completed"}) {
			t.Fatalf("recovery fixture missing result evidence:\n%s", plain)
		}
	}
	for _, semanticCase := range fixture.SemanticCases() {
		got := RenderSemanticJSON(fixture.State, semanticCase.size)
		assertSemanticSnapshot(t, fixture, semanticCase.file, got)
		var snapshot SemanticSnapshot
		if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
			t.Fatalf("unmarshal semantic snapshot: %v", err)
		}
		if snapshot.Recovery == nil || snapshot.Recovery.Decision == nil || snapshot.Recovery.Decision.Source != "autonomy_policy" {
			t.Fatalf("semantic recovery = %+v", snapshot.Recovery)
		}
	}
}

func mutationResultFixtureSizes() []fixtureSize {
	return []fixtureSize{{Name: "80x24", Width: 80, Height: 24}, {Name: "120x32", Width: 120, Height: 32}, {Name: "160x45", Width: 160, Height: 45}}
}

func loadMutationResultFixture(t *testing.T, name string) renderFixture {
	t.Helper()

	state := IdleEmptyState()
	state.Phase = "BUILD"
	state.PhaseSource = "workflow.fixture"
	state.Scenario = name
	state.RuntimeStatus = "idle"
	state.StatusSource = "runtime.dispatch"
	state.StatusDetail = "mutation tool dispatch"
	state.Autonomy = "write"
	switch name {
	case "mutation-success":
		state.RuntimeResult = "write internal/demo.txt: completed 12 bytes"
		state.Mutation = &MutationView{
			Name:                  "write",
			Status:                "completed",
			Path:                  "internal/demo.txt",
			ExpectedEffect:        "create demo file",
			PreviousVersion:       "missing",
			NewVersion:            "sha256:demo-new",
			PreviousExists:        false,
			BytesWritten:          12,
			ResolvedPathAvailable: true,
			Decision: &DecisionView{
				Autonomy:         "write",
				Source:           "autonomy_policy",
				Allowed:          true,
				Automatic:        true,
				ApprovalRequired: false,
				Reason:           "write autonomy allows classified operation",
				OperationKind:    "mutation",
				Name:             "write",
				Target:           "internal/demo.txt",
				ExpectedEffect:   "create demo file",
				Reversible:       false,
				RunID:            "run-write-fixture",
				Capability:       "mutation-fixture",
			},
		}
	case "mutation-failure":
		state.RuntimeResult = "edit internal/demo.txt failed: target_version_mismatch"
		state.Mutation = &MutationView{
			Name:                  "edit",
			Status:                "failed",
			Path:                  "internal/demo.txt",
			ExpectedEffect:        "replace demo text",
			PreviousVersion:       "",
			NewVersion:            "",
			PreviousExists:        false,
			BytesWritten:          0,
			ReplacementCount:      0,
			ResolvedPathAvailable: true,
			ErrorKind:             "target_version_mismatch",
			ErrorMessage:          "target version mismatch: expected sha256:old",
			Decision: &DecisionView{
				Autonomy:         "write",
				Source:           "autonomy_policy",
				Allowed:          true,
				Automatic:        true,
				ApprovalRequired: false,
				Reason:           "write autonomy allows classified operation",
				OperationKind:    "mutation",
				Name:             "edit",
				Target:           "internal/demo.txt",
				ExpectedEffect:   "replace demo text",
				Reversible:       true,
				RunID:            "run-edit-fixture",
				Capability:       "mutation-fixture",
			},
		}
	default:
		t.Fatalf("unknown mutation result fixture %q", name)
	}
	return loadRenderFixture(t, name, state)
}

func TestMutationResultRenderAndSemantic(t *testing.T) {
	for _, tc := range []struct {
		name      string
		tool      string
		status    string
		errorKind string
	}{
		{name: "mutation-success", tool: "write", status: "completed"},
		{name: "mutation-failure", tool: "edit", status: "failed", errorKind: "target_version_mismatch"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fixture := loadMutationResultFixture(t, tc.name)
			render := RenderPlain(fixture.State, Size{Width: 120, Height: 32})
			if !containsAll(render, []string{"Mutation result:", "tool: " + tc.tool, "status: " + tc.status, "path: internal/demo.txt", "decision source: autonomy_policy", "approval required: false", "autonomy: write", "operation: mutation"}) {
				t.Fatalf("mutation render missing evidence:\n%s", render)
			}
			if tc.errorKind != "" && !containsAll(render, []string{"error kind: " + tc.errorKind, "bytes written: 0"}) {
				t.Fatalf("mutation failure render missing error evidence:\n%s", render)
			}
			snapshot := Semantic(fixture.State, Size{Width: 120, Height: 32})
			if snapshot.Mutation == nil || snapshot.Mutation.Name != tc.tool || snapshot.Mutation.Status != tc.status || snapshot.Mutation.Path != "internal/demo.txt" || snapshot.Mutation.Decision == nil || snapshot.Mutation.Decision.Source != "autonomy_policy" || snapshot.Mutation.Decision.ApprovalRequired {
				t.Fatalf("mutation semantic = %+v", snapshot.Mutation)
			}
			regions := semanticRegionsByName(t, snapshot)
			mutationRegion := strings.Join(regions["mutation_tool"].Items, "\n")
			if !containsAll(mutationRegion, []string{"tool_name: " + tc.tool, "status: " + tc.status, "path: internal/demo.txt", "decision_source: autonomy_policy", "display-only"}) {
				t.Fatalf("mutation semantic region = %v", regions["mutation_tool"].Items)
			}
		})
	}
}

func TestMutationResultFixtureSnapshots(t *testing.T) {
	for _, name := range []string{"mutation-success", "mutation-failure"} {
		name := name
		t.Run(name, func(t *testing.T) {
			fixture := loadMutationResultFixture(t, name)
			assertFixtureSizes(t, fixture, mutationResultFixtureSizes())
			for _, renderCase := range fixture.TextCases() {
				got := trimSnapshotLinePadding(renderCase.render(fixture.State, renderCase.size))
				assertTextSnapshot(t, fixture, renderCase.file, got)
				plain := stripANSI(got)
				if !containsAll(plain, []string{"Mutation result:", "path: internal/demo.txt", "status:"}) {
					t.Fatalf("%s fixture missing mutation evidence:\n%s", name, plain)
				}
			}
			for _, semanticCase := range fixture.SemanticCases() {
				got := RenderSemanticJSON(fixture.State, semanticCase.size)
				assertSemanticSnapshot(t, fixture, semanticCase.file, got)
				var snapshot SemanticSnapshot
				if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
					t.Fatalf("unmarshal semantic snapshot: %v", err)
				}
				if snapshot.Mutation == nil || snapshot.Mutation.Decision == nil || snapshot.Mutation.Decision.Source != "autonomy_policy" {
					t.Fatalf("semantic mutation = %+v", snapshot.Mutation)
				}
			}
		})
	}
}
