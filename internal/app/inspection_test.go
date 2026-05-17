package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/history"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

func TestStatusCommandBuildsAppOwnedRuntimeInspectionSurface(t *testing.T) {
	t.Parallel()

	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	var dispatched [][]runtime.Effect
	var historyReads int
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return runtime.Dispatch(effects)
	})
	controller := newSessionControllerWithPersistenceHistoryReadAndDiff(context.Background(), snapshotTestView(), runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	}, func(context.Context, HistoryReadCommand) HistoryReadResult {
		historyReads++
		return HistoryReadResult{
			State: state.FakeHistoryLoaded,
			Events: []history.FakeEvent{{
				SchemaVersion: history.FakeEventSchemaVersion,
				Kind:          history.EventKindRuntime,
				EventID:       "event-status-1",
				RunID:         "run-status",
				SessionID:     "session-status",
				Source:        "runtime.dispatch",
				Provenance:    "runtime.dispatch",
				DisplayText:   "runtime idle before brief",
			}},
		}
	}, func(context.Context, DiffReadCommand) DiffReadResult {
		t.Fatal("status inspection must not read diff state")
		return DiffReadResult{}
	})

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteStatus, Kind: policy.CommandInputSlash}, controller.view)

	if got.SurfaceTitle != "status" || got.CommandRoute != "status" || got.RouteSource != "policy.command" {
		t.Fatalf("status command surface = title=%q route=%q source=%q", got.SurfaceTitle, got.CommandRoute, got.RouteSource)
	}
	lines := strings.Join(got.SurfaceLines, "\n")
	for _, want := range []string{
		"source: app.status",
		"read-only: true",
		"runtime source: runtime.dispatch",
		"runtime detail: brief capability status",
		"runtime result: Brief: phase idle, runtime idle, store initialized, history loaded (1 events), context unavailable (placeholder), health available.",
		"last command: status",
		"project store: initialized (state.open; project store ready)",
		"utility worker: completed",
		"utility source: app.status",
		"utility job: summary_refresh status-summary-refresh",
		"utility summary: summary refresh confidence low",
		"utility summary refresh: low_confidence",
		"utility original summary: Status output is available for the current runtime.",
		"utility refreshed summary: Status output is available for the current runtime. Important details: primary runtime remains idle; utility worker can refresh summaries without final judgment refs=summary-refresh-runtime,summary-refresh-roadmap",
		"utility summary refresh source refs: summary-refresh-runtime,summary-refresh-roadmap",
		"utility summary refresh confidence: low",
		"utility summary refresh detail: primary runtime remains idle",
		"utility summary refresh detail: utility worker can refresh summaries without final judgment",
		"utility summary refresh caveat: refresh confidence is low; foreground work must check source refs before using refreshed summary",
		"utility suggestion: Review preserved source refs before using the refreshed summary. refs=summary-refresh-source-1,summary-refresh-source-2,summary-refresh-detail-1,summary-refresh-detail-2",
		"utility evidence: summary-refresh-source-1 source_ref app.status source_ref=summary-refresh-runtime",
		"utility evidence: summary-refresh-detail-2 exact_detail app.status utility worker can refresh summaries without final judgment",
		"utility caveat: refresh confidence is low; foreground work must check source refs before using refreshed summary",
		"utility file mutation: false",
		"utility git mutation: false",
		"utility artifact mutation: false",
		"utility permission approval: false",
		"utility workflow transition: false",
		"utility final judgment: false",
		"utility context refresh: false",
		"utility context compaction: false",
		"utility context rewrite: false",
		"brief capability: brief complete",
		"brief source: app.brief",
		"brief current phase: idle",
		"brief runtime status: idle",
		"brief transition claimed: false",
		"brief display-only: true",
		"brief known gap: context unavailable",
		"brief suggested next action: Continue with the current roadmap task or choose the next capability.",
		"brief requested boundary: state_access operation=state.access target=runtime.current",
		"brief requested boundary: artifact_access operation=artifact.access target=fake_history",
		"brief source ref: brief-history kind=history excerpt=runtime runtime.dispatch runtime idle before brief",
		"inspection: app-owned display data",
	} {
		if !strings.Contains(lines, want) {
			t.Fatalf("status inspection missing %q in:\n%s", want, lines)
		}
	}
	for _, forbidden := range []string{"Deterministic placeholder status", "real status sources: deferred", "review", "provider review"} {
		if strings.Contains(lines, forbidden) {
			t.Fatalf("status inspection leaked forbidden marker %q in:\n%s", forbidden, lines)
		}
	}
	if historyReads != 1 {
		t.Fatalf("status history reads = %d, want one brief evidence read", historyReads)
	}
	if len(dispatched) != 3 || len(dispatched[0]) != 1 || len(dispatched[1]) != 1 || len(dispatched[2]) != 1 {
		t.Fatalf("status dispatches = %#v, want command effect, utility effect, then capability effect", dispatched)
	}
	if _, ok := dispatched[0][0].(runtime.FakeCommandEffect); !ok {
		t.Fatalf("status dispatch = %T, want runtime.FakeCommandEffect", dispatched[0][0])
	}
	if _, ok := dispatched[1][0].(runtime.UtilityJobEffect); !ok {
		t.Fatalf("utility dispatch = %T, want runtime.UtilityJobEffect", dispatched[1][0])
	}
	if _, ok := dispatched[2][0].(runtime.CapabilityEffect); !ok {
		t.Fatalf("capability dispatch = %T, want runtime.CapabilityEffect", dispatched[2][0])
	}
	if len(snapshots) != 1 || !strings.Contains(snapshots[0].Snapshot.Runtime.Result, "Brief: phase idle") || got.Utility == nil || got.Utility.Status != "completed" || got.Brief == nil || got.Brief.SuggestedNextAction == "" {
		t.Fatalf("status snapshots/view = %#v / utility=%+v brief=%+v, want one snapshot with brief result, completed utility view, and brief view", snapshots, got.Utility, got.Brief)
	}
	if len(historyEvents) != 2 || historyEvents[0].Event.Kind != history.EventKindCommand || historyEvents[1].Event.Kind != history.EventKindRuntime {
		t.Fatalf("status history events = %#v, want command then runtime event", historyEvents)
	}
}

func TestPlanCommandRunsCapabilityPersistsArtifactAndDisplaysPlan(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	view := snapshotTestView()
	view.Phase = "BUILD"
	view.PhaseSource = "build"
	view.RuntimeStatus = "idle"
	view.StatusSource = "runtime.dispatch"
	view.FooterContext = "Milestone 47 plan capability"
	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	var dispatched [][]runtime.Effect
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return runtime.Dispatch(effects)
	})
	controller := newSessionControllerWithPersistenceHistoryReadAndDiff(context.Background(), view, runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	}, nil, func(context.Context, DiffReadCommand) DiffReadResult {
		t.Fatal("plan command must not read diff state")
		return DiffReadResult{}
	})
	controller.workspacePath = workspace

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRoutePlan, Kind: policy.CommandInputSlash}, controller.view)

	if got.Plan == nil {
		t.Fatal("plan view is nil")
	}
	if got.SurfaceTitle != "" || got.CommandRoute != "plan" || got.RouteSource != "policy.command" {
		t.Fatalf("plan command surface = title=%q route=%q source=%q", got.SurfaceTitle, got.CommandRoute, got.RouteSource)
	}
	if got.Plan.ArtifactStatus != "written" || got.Plan.ArtifactPath == "" {
		t.Fatalf("plan artifact state = %+v", got.Plan)
	}
	content, err := os.ReadFile(got.Plan.ArtifactPath)
	if err != nil {
		t.Fatalf("read plan artifact: %v", err)
	}
	planText := string(content)
	for _, want := range []string{"# Current Session Plan", "Scope: Milestone 47 plan capability", "GIVEN implementation starts WHEN code changes are made", "Next action: Review the plan artifact"} {
		if !strings.Contains(planText, want) {
			t.Fatalf("plan artifact missing %q in:\n%s", want, planText)
		}
	}
	if len(got.Plan.Items) != 3 || !got.Plan.Items[0].Done || got.Plan.Items[1].Done {
		t.Fatalf("plan items = %+v", got.Plan.Items)
	}
	if got.Plan.RecommendedSuccessor != "plan" || !got.Plan.SuccessorValid || got.Plan.TransitionClaimed || !got.Plan.DisplayOnly {
		t.Fatalf("plan successor/display = %+v", got.Plan)
	}
	if len(dispatched) != 1 || len(dispatched[0]) != 1 {
		t.Fatalf("plan dispatches = %#v, want one capability effect", dispatched)
	}
	if _, ok := dispatched[0][0].(runtime.CapabilityEffect); !ok {
		t.Fatalf("plan dispatch = %T, want runtime.CapabilityEffect", dispatched[0][0])
	}
	if runner.model.LastCapability.Plan == nil || runner.model.LastCapability.RecommendedSuccessor != workflow.PhasePlan {
		t.Fatalf("runtime last capability = %+v", runner.model.LastCapability)
	}
	if len(snapshots) != 1 || snapshots[0].Snapshot.Runtime.Result == "" || snapshots[0].Snapshot.Runtime.Result != runner.model.Result {
		t.Fatalf("plan snapshots = %#v", snapshots)
	}
	if len(historyEvents) < 2 || historyEvents[0].Event.Kind != history.EventKindCommand || historyEvents[1].Event.Kind != history.EventKindRuntime {
		t.Fatalf("plan history events = %#v, want command then runtime", historyEvents)
	}
}

func TestVisionCommandRunsCapabilityPersistsArtifactAndDisplaysVision(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	view := snapshotTestView()
	view.RuntimeStatus = "idle"
	view.StatusSource = "runtime.dispatch"
	view.FooterContext = "Milestone 50 vision capability"
	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	var dispatched [][]runtime.Effect
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return runtime.Dispatch(effects)
	})
	controller := newSessionControllerWithPersistenceHistoryReadAndDiff(context.Background(), view, runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	}, nil, func(context.Context, DiffReadCommand) DiffReadResult {
		t.Fatal("vision command must not read diff state")
		return DiffReadResult{}
	})
	controller.workspacePath = workspace

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteVision, Kind: policy.CommandInputSlash}, controller.view)

	if got.Vision == nil {
		t.Fatal("vision view is nil")
	}
	if got.SurfaceTitle != "" || got.CommandRoute != "vision" || got.RouteSource != "policy.command" {
		t.Fatalf("vision command surface = title=%q route=%q source=%q", got.SurfaceTitle, got.CommandRoute, got.RouteSource)
	}
	if got.Vision.ArtifactStatus != "written" || got.Vision.ArtifactPath == "" {
		t.Fatalf("vision artifact state = %+v", got.Vision)
	}
	content, err := os.ReadFile(got.Vision.ArtifactPath)
	if err != nil {
		t.Fatalf("read vision artifact: %v", err)
	}
	visionText := string(content)
	for _, want := range []string{"# Vision", "North star: Shape Aila's project direction for Milestone 50 vision capability.", "## Principles", "Next action: Use this vision as source material for planning."} {
		if !strings.Contains(visionText, want) {
			t.Fatalf("vision artifact missing %q in:\n%s", want, visionText)
		}
	}
	if got.Vision.Phase != "envision" || got.Vision.RecommendedSuccessor != "plan" || !got.Vision.SuccessorValid || got.Vision.TransitionClaimed || !got.Vision.DisplayOnly {
		t.Fatalf("vision successor/display = %+v", got.Vision)
	}
	if len(got.Vision.Principles) == 0 || len(got.Vision.LongTermGoals) == 0 || got.Vision.NeededInput != "" {
		t.Fatalf("vision evidence = %+v", got.Vision)
	}
	if len(dispatched) != 1 || len(dispatched[0]) != 1 {
		t.Fatalf("vision dispatches = %#v, want one capability effect", dispatched)
	}
	if _, ok := dispatched[0][0].(runtime.CapabilityEffect); !ok {
		t.Fatalf("vision dispatch = %T, want runtime.CapabilityEffect", dispatched[0][0])
	}
	if runner.model.LastCapability.Vision == nil || runner.model.LastCapability.RecommendedSuccessor != workflow.PhasePlan {
		t.Fatalf("runtime last capability = %+v", runner.model.LastCapability)
	}
	if len(snapshots) != 1 || snapshots[0].Snapshot.Runtime.Result == "" || snapshots[0].Snapshot.Runtime.Result != runner.model.Result {
		t.Fatalf("vision snapshots = %#v", snapshots)
	}
	if len(historyEvents) < 2 || historyEvents[0].Event.Kind != history.EventKindCommand || historyEvents[1].Event.Kind != history.EventKindRuntime {
		t.Fatalf("vision history events = %#v, want command then runtime", historyEvents)
	}
}

func TestDiscussCommandRunsCapabilityPersistsArtifactAndDisplaysDecision(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	view := snapshotTestView()
	view.Phase = "DELIBERATE"
	view.PhaseSource = "deliberate"
	view.RuntimeStatus = "idle"
	view.StatusSource = "runtime.dispatch"
	view.FooterContext = "Milestone 50A discuss capability"
	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	var dispatched [][]runtime.Effect
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return runtime.Dispatch(effects)
	})
	controller := newSessionControllerWithPersistenceHistoryReadAndDiff(context.Background(), view, runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	}, nil, func(context.Context, DiffReadCommand) DiffReadResult {
		t.Fatal("discuss command must not read diff state")
		return DiffReadResult{}
	})
	controller.workspacePath = workspace

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteDiscuss, Kind: policy.CommandInputSlash}, controller.view)

	if got.Discuss == nil {
		t.Fatal("discuss view is nil")
	}
	if got.SurfaceTitle != "" || got.CommandRoute != "discuss" || got.RouteSource != "policy.command" {
		t.Fatalf("discuss command surface = title=%q route=%q source=%q", got.SurfaceTitle, got.CommandRoute, got.RouteSource)
	}
	if got.Discuss.ArtifactStatus != "written" || got.Discuss.ArtifactPath == "" {
		t.Fatalf("discuss artifact state = %+v", got.Discuss)
	}
	content, err := os.ReadFile(got.Discuss.ArtifactPath)
	if err != nil {
		t.Fatalf("read decision artifact: %v", err)
	}
	decisionText := string(content)
	for _, want := range []string{"# Decision", "Question: Decide the next safe workflow direction for Milestone 50A discuss capability.", "## Options", "Choice: Plan the scoped next step", "Next action: Use this decision as source material for planning."} {
		if !strings.Contains(decisionText, want) {
			t.Fatalf("decision artifact missing %q in:\n%s", want, decisionText)
		}
	}
	if got.Discuss.Phase != "deliberate" || got.Discuss.RecommendedSuccessor != "plan" || !got.Discuss.SuccessorValid || got.Discuss.TransitionClaimed || !got.Discuss.DisplayOnly {
		t.Fatalf("discuss successor/display = %+v", got.Discuss)
	}
	if len(got.Discuss.Options) != 3 || !got.Discuss.Options[0].Selected || got.Discuss.Selected != "Plan the scoped next step" || got.Discuss.NeededInput != "" {
		t.Fatalf("discuss evidence = %+v", got.Discuss)
	}
	if len(dispatched) != 1 || len(dispatched[0]) != 1 {
		t.Fatalf("discuss dispatches = %#v, want one capability effect", dispatched)
	}
	if _, ok := dispatched[0][0].(runtime.CapabilityEffect); !ok {
		t.Fatalf("discuss dispatch = %T, want runtime.CapabilityEffect", dispatched[0][0])
	}
	if runner.model.LastCapability.Discuss == nil || runner.model.LastCapability.RecommendedSuccessor != workflow.PhasePlan {
		t.Fatalf("runtime last capability = %+v", runner.model.LastCapability)
	}
	if len(snapshots) != 1 || snapshots[0].Snapshot.Runtime.Result == "" || snapshots[0].Snapshot.Runtime.Result != runner.model.Result {
		t.Fatalf("discuss snapshots = %#v", snapshots)
	}
	if len(historyEvents) < 2 || historyEvents[0].Event.Kind != history.EventKindCommand || historyEvents[1].Event.Kind != history.EventKindRuntime {
		t.Fatalf("discuss history events = %#v, want command then runtime", historyEvents)
	}
}

func TestResearchCommandRunsCapabilityAndFoldsContextWithoutArtifactWrite(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	view := snapshotTestView()
	view.Phase = "BUILD"
	view.PhaseSource = "build"
	view.RuntimeStatus = "idle"
	view.StatusSource = "runtime.dispatch"
	view.FooterContext = "Milestone 51 research capability"
	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	var dispatched [][]runtime.Effect
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return runtime.Dispatch(effects)
	})
	controller := newSessionControllerWithPersistenceHistoryReadAndDiff(context.Background(), view, runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	}, nil, func(context.Context, DiffReadCommand) DiffReadResult {
		t.Fatal("research command must not read diff state")
		return DiffReadResult{}
	})
	controller.workspacePath = workspace

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteResearch, Kind: policy.CommandInputSlash}, controller.view)

	if got.Research == nil {
		t.Fatal("research view is nil")
	}
	if got.SurfaceTitle != "" || got.CommandRoute != "research" || got.RouteSource != "policy.command" {
		t.Fatalf("research command surface = title=%q route=%q source=%q", got.SurfaceTitle, got.CommandRoute, got.RouteSource)
	}
	if got.Research.CurrentPhase != workflow.PhaseBuild.String() || got.Research.RecommendedSuccessor != "" || got.Research.TransitionClaimed || !got.Research.DisplayOnly || !got.Research.ContextFolded {
		t.Fatalf("research transition/display = %+v", got.Research)
	}
	if got.Context == nil || got.Context.Source != "app.research.context" || got.Context.Status != "folded" || len(got.Context.Claims) == 0 || len(got.Context.SourceRefs) == 0 {
		t.Fatalf("research context = %+v", got.Context)
	}
	if len(got.Research.Patterns) != 3 || len(got.Research.Evidence) != 3 || got.Research.Confidence != "medium" || len(got.Research.Caveats) != 2 {
		t.Fatalf("research evidence = %+v", got.Research)
	}
	if len(dispatched) != 1 || len(dispatched[0]) != 1 {
		t.Fatalf("research dispatches = %#v, want one capability effect", dispatched)
	}
	if _, ok := dispatched[0][0].(runtime.CapabilityEffect); !ok {
		t.Fatalf("research dispatch = %T, want runtime.CapabilityEffect", dispatched[0][0])
	}
	if runner.model.LastCapability.Research == nil || runner.model.LastCapability.RecommendedSuccessor != "" {
		t.Fatalf("runtime last capability = %+v", runner.model.LastCapability)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".aila", "artifacts", "research.md")); !os.IsNotExist(err) {
		t.Fatalf("research artifact should not exist, stat err=%v", err)
	}
	if len(snapshots) != 1 || snapshots[0].Snapshot.Runtime.Result == "" || snapshots[0].Snapshot.Runtime.Result != runner.model.Result {
		t.Fatalf("research snapshots = %#v", snapshots)
	}
	if len(historyEvents) < 2 || historyEvents[0].Event.Kind != history.EventKindCommand || historyEvents[1].Event.Kind != history.EventKindRuntime {
		t.Fatalf("research history events = %#v, want command then runtime", historyEvents)
	}
}

func TestReviewCommandRunsReadOnlyAuditOverDiffAndHistoryInspectionSurface(t *testing.T) {
	t.Parallel()

	view := snapshotTestView()
	view.RuntimeStatus = "idle"
	view.RuntimeResult = "stable before review"
	var snapshots []SnapshotPersistenceCommand
	var historyEvents []HistoryPersistenceCommand
	var dispatched [][]runtime.Effect
	var historyReads int
	var diffReads int
	runner := newInputRunnerWithDispatch(func(effects []runtime.Effect) []runtime.Message {
		dispatched = append(dispatched, append([]runtime.Effect(nil), effects...))
		return runtime.Dispatch(effects)
	})
	controller := newSessionControllerWithPersistenceHistoryReadAndDiff(context.Background(), view, runner, func(_ context.Context, command SnapshotPersistenceCommand) SnapshotPersistenceResult {
		snapshots = append(snapshots, command)
		return SnapshotPersistenceResult{}
	}, func(_ context.Context, command HistoryPersistenceCommand) HistoryPersistenceResult {
		historyEvents = append(historyEvents, command)
		return HistoryPersistenceResult{}
	}, func(context.Context, HistoryReadCommand) HistoryReadResult {
		historyReads++
		return HistoryReadResult{
			State: state.FakeHistoryLoaded,
			Events: []history.FakeEvent{{
				SchemaVersion: history.FakeEventSchemaVersion,
				Kind:          history.EventKindMutation,
				EventID:       "evt-review-1",
				RunID:         "run-review",
				SessionID:     "session-review",
				Source:        "mutation.tool",
				Provenance:    "mutation.result",
				DisplayText:   "mutation write completed docs/review.md",
				Mutation: &history.MutationRecord{
					ToolName:     "write",
					Status:       "completed",
					ChangedPaths: []string{"docs/review.md"},
				},
				Undo: &history.UndoMetadata{
					Available: true,
					Action:    "restore_file",
					Paths:     []string{"docs/review.md"},
				},
			}},
		}
	}, func(context.Context, DiffReadCommand) DiffReadResult {
		diffReads++
		return DiffReadResult{View: &tui.DiffView{Source: "test.diff", Status: "ready", Files: []tui.DiffFileView{
			{Path: "internal/demo.txt", Status: "modified"},
			{Path: "docs/review.md", Status: "added"},
		}}}
	})

	got := controller.routeCommand(policy.CommandRecommendation{Route: policy.CommandRouteReview, Kind: policy.CommandInputSlash}, controller.view)

	if historyReads != 1 || diffReads != 1 {
		t.Fatalf("review reads = history:%d diff:%d, want one each", historyReads, diffReads)
	}
	if got.SurfaceTitle != "review" || got.CommandRoute != "review" || got.RouteSource != "policy.command" {
		t.Fatalf("review command surface = title=%q route=%q source=%q", got.SurfaceTitle, got.CommandRoute, got.RouteSource)
	}
	if got.HistoryFocus || got.DiffFocus {
		t.Fatalf("review mutated unrelated focus state: before=%+v after=%+v", view, got)
	}
	if got.Audit == nil || got.Audit.Capability != "audit" || got.Audit.Signal != "flagged" || got.Audit.EvidenceState != "diff_available" || !got.Audit.SuccessorValid || got.Audit.TransitionClaimed || !got.Audit.DisplayOnly {
		t.Fatalf("audit view = %+v", got.Audit)
	}
	if got.RuntimeResult != "Audit found 2 changed file(s) needing review." {
		t.Fatalf("review runtime result = %q", got.RuntimeResult)
	}
	lines := strings.Join(got.SurfaceLines, "\n")
	for _, want := range []string{
		"source: app.review",
		"read-only: true",
		"model-assisted review: not invoked",
		"diff source: test.diff",
		"diff status: ready",
		"changed files: 2",
		"changed file: internal/demo.txt status=modified",
		"changed file: docs/review.md status=added",
		"history state: loaded",
		"history events: 1",
		"latest event: mutation mutation.tool mutation write completed docs/review.md",
		"latest mutation: write completed docs/review.md",
		"latest undo action: restore_file",
		"attention: inspect changed files before committing",
		"inspection: app-owned display data",
	} {
		if !strings.Contains(lines, want) {
			t.Fatalf("review inspection missing %q in:\n%s", want, lines)
		}
	}
	auditPlain := tui.RenderPlain(got, tui.Size{Width: 120, Height: 32})
	for _, want := range []string{
		"Audit:",
		"capability: audit",
		"signal: flagged",
		"evidence: diff_available",
		"finding: current-change-review severity=warning title=Review current changes before continuing",
		"finding source refs: current-change-review review-diff",
		"finding next action: current-change-review Route back to build after reviewing changed files.",
		"successor valid: true",
		"transition claimed: false",
		"display-only: true",
	} {
		if !strings.Contains(auditPlain, want) {
			t.Fatalf("review audit render missing %q in:\n%s", want, auditPlain)
		}
	}
	if strings.Contains(lines, "provider") || strings.Contains(auditPlain, "provider") {
		t.Fatalf("review inspection claimed provider work:\n%s\n---\n%s", lines, auditPlain)
	}
	if len(dispatched) != 1 || len(dispatched[0]) != 1 {
		t.Fatalf("review dispatches = %#v, want one audit capability effect", dispatched)
	}
	if len(historyEvents) < 2 || historyEvents[0].Event.Kind != history.EventKindCommand || historyEvents[1].Event.Kind != history.EventKindRuntime {
		t.Fatalf("review history events = %#v, want command then runtime", historyEvents)
	}
	if len(snapshots) != 1 || snapshots[0].Snapshot.Runtime.Result != "Audit found 2 changed file(s) needing review." {
		t.Fatalf("review snapshots = %#v, want one snapshot with audit result", snapshots)
	}
}
